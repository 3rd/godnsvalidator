package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type Resolver struct {
	Address string
	Time    time.Duration
}

func main() {
	serversFile := flag.String("servers", "", "File containing the list of DNS servers")
	threads := flag.Int("threads", 20, "Number of concurrent threads")
	timeout := flag.Int("timeout", 200, "Timeout in milliseconds for connect")
	rootDomain := flag.String("root", "example.com", "Root domain to resolve")
	outputFile := flag.String("o", "", "Output file to write the sorted resolvers")
	maxResponseTime := flag.Int("max", 0, "Maximum response time in milliseconds")
	flag.Parse()

	if *serversFile == "" {
		fmt.Println("Please provide the required flag: -servers")
		os.Exit(1)
	}

	servers, err := readServers(*serversFile)
	if err != nil {
		fmt.Printf("Error reading servers file: %v\n", err)
		os.Exit(1)
	}

	hardcodedResolver := "1.1.1.1:53"
	expectedIP, err := resolve(hardcodedResolver, *rootDomain, 5*time.Second)
	if err != nil {
		fmt.Printf("Error resolving IP using hardcoded resolver: %v\n", err)
		os.Exit(1)
	}

	resolvers := validateResolvers(servers, *rootDomain, expectedIP, *threads, time.Duration(*timeout)*time.Millisecond, time.Duration(*maxResponseTime)*time.Millisecond)

	printResolvers(resolvers)

	if *outputFile != "" {
		err := writeResolvers(resolvers, *outputFile)
		if err != nil {
			fmt.Printf("Error writing resolvers to file: %v\n", err)
		}
	}
}

func readServers(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var servers []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		servers = append(servers, scanner.Text())
	}

	return servers, scanner.Err()
}

func resolve(resolver, domain string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		dialer := &net.Dialer{}
		conn, err := dialer.DialContext(ctx, "udp", resolver)
		if err != nil {
			errChan <- err
			return
		}
		defer conn.Close()

		dnsConn := &dns.Conn{Conn: conn}
		defer dnsConn.Close()

		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)

		err = dnsConn.WriteMsg(msg)
		if err != nil {
			errChan <- err
			return
		}

		resp, err := dnsConn.ReadMsg()
		if err != nil {
			errChan <- err
			return
		}

		if resp.Rcode != dns.RcodeSuccess {
			errChan <- fmt.Errorf("%s", dns.RcodeToString[resp.Rcode])
			return
		}

		if len(resp.Answer) == 0 {
			errChan <- fmt.Errorf("no answer")
			return
		}

		a, ok := resp.Answer[0].(*dns.A)
		if !ok {
			errChan <- fmt.Errorf("failed to cast to dns.A")
			return
		}
		resultChan <- a.A.String()
	}()

	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("timeout exceeded")
	}
}

func validateResolvers(servers []string, rootDomain, expectedIP string, threads int, timeout time.Duration, maxResponseTime time.Duration) []Resolver {
	var resolvers []Resolver
	var mu sync.Mutex
	var wg sync.WaitGroup

	workChan := make(chan string, len(servers))

	for _, server := range servers {
		workChan <- server
	}
	close(workChan)

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for server := range workChan {
				start := time.Now()
				ip, err := resolve(server+":53", rootDomain, timeout)
				elapsed := time.Since(start)

				mu.Lock()
				if err == nil {
					if ip == expectedIP {
						resolvers = append(resolvers, Resolver{Address: server, Time: elapsed})
						fmt.Printf("Resolver %s: %s (correct)\n", server, elapsed)
					} else {
						fmt.Printf("Resolver %s: %s (incorrect)\n", server, elapsed)
					}
				} else {
					fmt.Printf("Resolver %s: failed (%v)\n", server, err)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	sort.Slice(resolvers, func(i, j int) bool {
		return resolvers[i].Time < resolvers[j].Time
	})

	if maxResponseTime > 0 {
		for i := 0; i < len(resolvers); i++ {
			if resolvers[i].Time > maxResponseTime {
				resolvers = resolvers[:i]
				break
			}
		}
	}

	return resolvers
}

func printResolvers(resolvers []Resolver) {
	fmt.Println("\nWorking resolvers (sorted by speed):")
	for _, resolver := range resolvers {
		fmt.Printf("%s: %s\n", resolver.Address, resolver.Time)
	}
}

func writeResolvers(resolvers []Resolver, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, resolver := range resolvers {
		_, err := fmt.Fprintf(writer, "%s\n", resolver.Address)
		if err != nil {
			return err
		}
	}

	return writer.Flush()
}
