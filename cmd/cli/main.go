package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mhq-projects/web-crawler/internal/config"
	"github.com/mhq-projects/web-crawler/internal/frontier"
	"github.com/mhq-projects/web-crawler/internal/storage"
	"github.com/mhq-projects/web-crawler/pkg/models"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "crawler-cli",
	Short: "Web Crawler CLI",
	Long:  "Command-line interface for managing the web crawler",
}

func init() {
	rootCmd.AddCommand(seedCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(queueCmd)
}

// Seed command
var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Add seed URLs to the crawler",
}

var seedURLFlag string
var seedFileFlag string
var seedPriorityFlag int
var seedDepthFlag int

func init() {
	seedCmd.AddCommand(seedAddCmd)
	seedAddCmd.Flags().StringVarP(&seedURLFlag, "url", "u", "", "Single URL to add")
	seedAddCmd.Flags().StringVarP(&seedFileFlag, "file", "f", "", "File containing URLs (one per line)")
	seedAddCmd.Flags().IntVarP(&seedPriorityFlag, "priority", "p", 0, "URL priority (0=highest)")
	seedAddCmd.Flags().IntVarP(&seedDepthFlag, "depth", "d", 0, "Starting depth")
}

var seedAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add seed URLs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if seedURLFlag == "" && seedFileFlag == "" {
			return fmt.Errorf("either --url or --file required")
		}

		cfg := config.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		redisClient, err := storage.NewRedisClient(cfg.Redis)
		if err != nil {
			return fmt.Errorf("connect to redis: %w", err)
		}
		defer redisClient.Close()

		front, err := frontier.New(redisClient, cfg.Frontier)
		if err != nil {
			return fmt.Errorf("create frontier: %w", err)
		}

		var urls []*models.URL

		if seedURLFlag != "" {
			u, err := models.NewURL(seedURLFlag, seedDepthFlag, seedPriorityFlag, "")
			if err != nil {
				return fmt.Errorf("invalid url %s: %w", seedURLFlag, err)
			}
			urls = append(urls, u)
		}

		if seedFileFlag != "" {
			file, err := os.Open(seedFileFlag)
			if err != nil {
				return fmt.Errorf("open file: %w", err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				u, err := models.NewURL(line, seedDepthFlag, seedPriorityFlag, "")
				if err != nil {
					fmt.Fprintf(os.Stderr, "line %d: invalid url %s: %v\n", lineNum, line, err)
					continue
				}
				urls = append(urls, u)
			}

			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read file: %w", err)
			}
		}

		if len(urls) == 0 {
			return fmt.Errorf("no valid urls to add")
		}

		added, err := front.AddURLs(ctx, urls)
		if err != nil {
			return fmt.Errorf("add urls: %w", err)
		}

		fmt.Printf("Added %d URLs (submitted: %d, skipped: %d)\n", added, len(urls), len(urls)-added)
		return nil
	},
}

// Stats command
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show crawler statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		redisClient, err := storage.NewRedisClient(cfg.Redis)
		if err != nil {
			return fmt.Errorf("connect to redis: %w", err)
		}
		defer redisClient.Close()

		front, err := frontier.New(redisClient, cfg.Frontier)
		if err != nil {
			return fmt.Errorf("create frontier: %w", err)
		}

		stats, err := front.Stats(ctx)
		if err != nil {
			return fmt.Errorf("get stats: %w", err)
		}

		fmt.Println("Crawler Statistics")
		fmt.Println("==================")
		fmt.Printf("Pending URLs:    %d\n", stats.TotalPending)
		fmt.Printf("Completed URLs:  %d\n", stats.TotalCompleted)
		fmt.Printf("Failed URLs:     %d\n", stats.TotalFailed)
		fmt.Printf("Active Hosts:    %d\n", len(stats.HostQueueCounts))

		if len(stats.HostQueueCounts) > 0 && len(stats.HostQueueCounts) <= 20 {
			fmt.Println("\nHost Queue Depths:")
			for host, count := range stats.HostQueueCounts {
				fmt.Printf("  %s: %d\n", host, count)
			}
		}

		// Try to get OpenSearch stats
		osClient, err := storage.NewOpenSearchClient(cfg.OpenSearch)
		if err == nil {
			count, err := osClient.GetStats(ctx)
			if err == nil {
				fmt.Printf("\nIndexed Pages:   %d\n", count)
			}
		}

		return nil
	},
}

// Search command
var searchQueryFlag string
var searchDomainFlag string
var searchLimitFlag int

func init() {
	searchCmd.Flags().StringVarP(&searchQueryFlag, "query", "q", "", "Search query")
	searchCmd.Flags().StringVarP(&searchDomainFlag, "domain", "d", "", "Filter by domain")
	searchCmd.Flags().IntVarP(&searchLimitFlag, "limit", "l", 10, "Number of results")
	searchCmd.MarkFlagRequired("query")
}

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search crawled content",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		osClient, err := storage.NewOpenSearchClient(cfg.OpenSearch)
		if err != nil {
			return fmt.Errorf("connect to opensearch: %w", err)
		}

		opts := models.SearchOpts{
			Query:  searchQueryFlag,
			Domain: searchDomainFlag,
			Size:   searchLimitFlag,
		}

		results, total, err := osClient.Search(ctx, opts)
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}

		fmt.Printf("Found %d results (showing %d):\n\n", total, len(results))

		for i, r := range results {
			fmt.Printf("%d. %s\n", i+1, r.Title)
			fmt.Printf("   URL: %s\n", r.URL)
			fmt.Printf("   Domain: %s\n", r.Domain)
			if r.Snippet != "" {
				fmt.Printf("   %s\n", r.Snippet)
			}
			fmt.Println()
		}

		return nil
	},
}

// Queue command
var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the URL queue",
}

func init() {
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueClearCmd)
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List queue status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		redisClient, err := storage.NewRedisClient(cfg.Redis)
		if err != nil {
			return fmt.Errorf("connect to redis: %w", err)
		}
		defer redisClient.Close()

		front, err := frontier.New(redisClient, cfg.Frontier)
		if err != nil {
			return fmt.Errorf("create frontier: %w", err)
		}

		stats, err := front.Stats(ctx)
		if err != nil {
			return fmt.Errorf("get stats: %w", err)
		}

		output, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(output))

		return nil
	},
}

var queueClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the queue (dangerous!)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print("Are you sure you want to clear the queue? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" {
			fmt.Println("Aborted")
			return nil
		}

		cfg := config.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		redisClient, err := storage.NewRedisClient(cfg.Redis)
		if err != nil {
			return fmt.Errorf("connect to redis: %w", err)
		}
		defer redisClient.Close()

		// Get all frontier keys
		keys, err := redisClient.Keys(ctx, "frontier:*")
		if err != nil {
			return fmt.Errorf("get keys: %w", err)
		}

		if len(keys) > 0 {
			if err := redisClient.Del(ctx, keys...); err != nil {
				return fmt.Errorf("delete keys: %w", err)
			}
		}

		// Clear seen URLs
		seenKeys, err := redisClient.Keys(ctx, "seen:*")
		if err == nil && len(seenKeys) > 0 {
			redisClient.Del(ctx, seenKeys...)
		}

		fmt.Printf("Cleared %d queue keys\n", len(keys))
		return nil
	},
}
