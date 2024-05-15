package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	maxConcurrency = 5
	maxRetries     = 3
	retryInterval  = 2 * time.Second
	logFileName    = "status_checker.log"
)

func main() {
	if err := os.Remove(logFileName); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove log file: %v", err)
	}

	logFile, err := os.Create(logFileName)
	if err != nil {
		log.Fatalf("Failed to create log file: %v", err)
	}

	defer logFile.Close()

	multi := io.MultiWriter(os.Stdout, logFile)
	logger := log.New(multi, "", log.LstdFlags)

	log.SetOutput(logger.Writer())

	links := []string{
		"http://google.com",
		"http://takis.gr",
		"http://facebook.com",
		"http://stackoverflow.com",
		"http://golang.org",
		"http://amazon.com",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	c := make(chan string)

	var wg sync.WaitGroup

	sem := make(chan struct{}, maxConcurrency)

	for _, link := range links {
		wg.Add(1)
		go checkLink(ctx, c, sem, &wg, link, 0)
	}

	go func() {
		<-sigs
		log.Println("Received shutdown signal, cancelling context...")

		cancel()
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case link := <-c:
				wg.Add(1)

				go func(link string) {
					time.Sleep(3 * time.Second)
					checkLink(ctx, c, sem, &wg, link, 0)
				}(link)
			}
		}
	}()

	wg.Wait()
	log.Println("All links checked, exiting.")
}

func checkLink(ctx context.Context, c chan string, sem chan struct{}, wg *sync.WaitGroup, link string, retryCount int) {
	defer wg.Done()

	sem <- struct{}{}
	defer func() { <-sem }()

	select {
	case <-ctx.Done():
		log.Printf("Context cancelled, stopping check for %s", link)
		return
	default:
	}

	resp, err := http.Get(link)
	if err != nil {
		log.Printf("Error checking link %s: %s", link, err)

		if retryCount < maxRetries {
			log.Printf("Retrying %s in %v (retry %d/%d)", link, retryInterval, retryCount+1, maxRetries)
			time.Sleep(retryInterval)
			checkLink(ctx, c, sem, wg, link, retryCount+1)
		} else {
			log.Printf("Max retries reached for %s, giving up.", link)
		}

		c <- link
		return
	}

	defer resp.Body.Close()

	log.Printf("%s is up, status code: %d", link, resp.StatusCode)
	c <- link
}
