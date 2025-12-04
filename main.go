package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type CRTEntry struct {
	NameValue string `json:"name_value"`
}

func trimSpaces(s string) string {
	return strings.TrimSpace(s)
}

func isCommentOrEmpty(line string) bool {
	line = strings.TrimSpace(line)
	return line == "" || strings.HasPrefix(line, "#")
}

// fetchCrtForDomain queries crt.sh for a given domain, extracts subdomains and wildcard roots,
// and enqueues new wildcard roots for further processing.
func fetchCrtForDomain(
	client *http.Client,
	current string,
	rateLimit time.Duration,
	maxRetries int,
	subsSet map[string]struct{},
	wildcardsSet map[string]struct{},
	seen map[string]struct{},
	queue *[]string,
) {
	fmt.Printf("    [*] Querying crt.sh for *.%s\n", current)

	url := fmt.Sprintf("https://crt.sh/?q=%%25.%s&output=json", current)

	var lastStatus int
	var body []byte
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var resp *http.Response
		resp, err = client.Get(url)
		if err != nil {
			fmt.Printf("    [!] Error requesting %s (attempt %d/%d): %v\n", current, attempt, maxRetries, err)
		} else {
			lastStatus = resp.StatusCode
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				fmt.Printf("    [!] Error reading response for %s (attempt %d/%d): %v\n", current, attempt, maxRetries, err)
			} else if resp.StatusCode == http.StatusOK {
				break
			} else {
				fmt.Printf("    [!] HTTP %d for %s (attempt %d/%d)\n", resp.StatusCode, current, attempt, maxRetries)
			}
		}
		time.Sleep(rateLimit)
	}

	if err != nil || lastStatus != http.StatusOK {
		fmt.Printf("    [!] Giving up on %s\n", current)
		return
	}

	// Parse JSON; crt.sh sometimes returns "[]" when no results
	var entries []CRTEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		fmt.Printf("    [!] Invalid JSON from crt.sh for %s (skipping): %v\n", current, err)
		return
	}

	if len(entries) == 0 {
		fmt.Printf("    [*] No results for %s\n", current)
		return
	}

	// Deduplicate name values
	namesSeen := make(map[string]struct{})

	for _, e := range entries {
		if e.NameValue == "" {
			continue
		}
		// name_value can contain multiple lines (multiple CNs)
		for _, raw := range strings.Split(e.NameValue, "\n") {
			name := strings.TrimSpace(strings.Trim(raw, "\r"))
			if name == "" {
				continue
			}
			if _, ok := namesSeen[name]; ok {
				continue
			}
			namesSeen[name] = struct{}{}

			if strings.HasPrefix(name, "*.") {
				// Clean wildcard: "*.ae.aliexpress.com" -> "ae.aliexpress.com"
				clean := strings.TrimPrefix(name, "*.")
				if clean == "" {
					continue
				}
				// Store wildcard root
				if _, ok := wildcardsSet[clean]; !ok {
					wildcardsSet[clean] = struct{}{}
				}
				// Enqueue for further processing if not already seen
				if _, ok := seen[clean]; !ok {
					*queue = append(*queue, clean)
				}
			} else {
				// Normal subdomain
				if _, ok := subsSet[name]; !ok {
					subsSet[name] = struct{}{}
				}
			}
		}
	}

	time.Sleep(rateLimit)
}

func processDomain(
	domain string,
	client *http.Client,
	rateLimit time.Duration,
	maxRetries int,
	skipDone bool,
) error {
	fmt.Printf("[+] Processing %s\n", domain)

	// Make directory for this domain
	if err := os.MkdirAll(domain, 0o755); err != nil {
		return fmt.Errorf("failed to create directory '%s': %w", domain, err)
	}

	subsPath := filepath.Join(domain, "subs.txt")
	wildcardsPath := filepath.Join(domain, "wildcards_clean.txt")

	// If skipDone is enabled and subs.txt exists and is non-empty, skip
	if skipDone {
		if info, err := os.Stat(subsPath); err == nil && info.Size() > 0 {
			fmt.Printf("[*] Skipping %s (subs.txt already exists)\n\n", domain)
			return nil
		}
	}

	// Sets for deduplication
	subsSet := make(map[string]struct{})
	wildcardsSet := make(map[string]struct{})

	seen := make(map[string]struct{})
	queue := []string{domain}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}

		fetchCrtForDomain(
			client,
			current,
			rateLimit,
			maxRetries,
			subsSet,
			wildcardsSet,
			seen,
			&queue,
		)
	}

	// Write subs.txt (sorted, unique)
	if err := writeSetSorted(subsPath, subsSet); err != nil {
		return fmt.Errorf("failed to write subs.txt for %s: %w", domain, err)
	}

	// Write wildcards_clean.txt (sorted, unique)
	if err := writeSetSorted(wildcardsPath, wildcardsSet); err != nil {
		return fmt.Errorf("failed to write wildcards_clean.txt for %s: %w", domain, err)
	}

	fmt.Printf("[+] Done → %s/\n\n", domain)
	return nil
}

func writeSetSorted(path string, set map[string]struct{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var items []string
	for k := range set {
		items = append(items, k)
	}
	sort.Strings(items)

	for _, v := range items {
		if _, err := fmt.Fprintln(f, v); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	// Flags
	rateLimitSec := flag.Int("rate", 1, "delay in seconds between crt.sh requests")
	maxRetries := flag.Int("retries", 3, "maximum retry attempts for each request")
	skipDone := flag.Bool("skip-done", true, "skip domains where subs.txt already exists and is non-empty")
	workers := flag.Int("workers", 1, "number of concurrent workers (1 = no concurrency)")
	timeoutSec := flag.Int("timeout", 20, "HTTP client timeout in seconds")

	flag.Parse()

	// Input file: first non-flag arg or default "domains.txt"
	inputFile := "domains.txt"
	if flag.NArg() > 0 {
		inputFile = flag.Arg(0)
	}

	// Check input file exists
	if _, err := os.Stat(inputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: input file '%s' not found.\n", inputFile)
		os.Exit(1)
	}

	rateLimit := time.Duration(*rateLimitSec) * time.Second

	client := &http.Client{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	}

	// Read domains first
	f, err := os.Open(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not open '%s': %v\n", inputFile, err)
		os.Exit(1)
	}
	defer f.Close()

	var domains []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if isCommentOrEmpty(line) {
			continue
		}
		domain := trimSpaces(line)
		if domain == "" {
			continue
		}
		domains = append(domains, domain)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading '%s': %v\n", inputFile, err)
	}

	if len(domains) == 0 {
		fmt.Println("No domains to process.")
		return
	}

	// If workers <= 1, run sequentially
	if *workers <= 1 {
		for _, domain := range domains {
			if err := processDomain(domain, client, rateLimit, *maxRetries, *skipDone); err != nil {
				fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", domain, err)
			}
		}
		return
	}

	// Concurrent processing with a worker pool
	fmt.Printf("Using %d workers\n", *workers)

	domainCh := make(chan string)
	var wg sync.WaitGroup

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for domain := range domainCh {
				if err := processDomain(domain, client, rateLimit, *maxRetries, *skipDone); err != nil {
					fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", domain, err)
				}
			}
		}()
	}

	for _, d := range domains {
		domainCh <- d
	}
	close(domainCh)
	wg.Wait()
}
