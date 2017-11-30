package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"
	"time"
)

type config struct {
	latency time.Duration
	verbose bool
	tls     tls.Config
}

type handler struct {
	s string
	f func(*net.Conn, []byte, *config)
}

var responses = map[string]handler{
	"EHLO":           {"250-Pleased to meet you!\r\n250-PIPELINING\r\n250-CHUNKING\r\n250-STARTTLS\r\n250 OK\r\n", nil},
	"LHLO":           {"250-Pleased to meet you!\r\n250-PIPELINING\r\n250-CHUNKING\r\n250-STARTTLS\r\n250 OK\r\n", nil},
	"HELO":           {"250 Pleased to meet you!\r\n", nil},
	"STAR" /*TTLS*/ : {"220 Ready to start TLS\r\n", handleStarttls},
	"MAIL":           {"250 OK\r\n", nil},
	"RCPT":           {"250 OK\r\n", nil},
	"DATA":           {"354 End data with <CR><LF>.<CR><LF>\r\n", handleData}, // Need to read data until \r\n.\r\n is received.
	"BDAT":           {"250 OK\r\n", handleBdat},                              // Should be sent once the data has been reveived
	"RSET":           {"250 OK\r\n", nil},
	"QUIT":           {"221 Goodbye\r\n", nil},
}

func sendResponse(c *net.Conn, s string, verbose bool) {
	(*c).Write([]byte(s))
	if verbose {
		strs := strings.Split(s, "\r\n")
		for _, str := range strs {
			if len(str) != 0 {
				log.Printf("<- [%s]", str)
			}
		}
	}
}

func handleConnection(c *net.Conn, conf *config) {
	// Print banner
	sendResponse(c, "220 Welcome to Blackhole SMTP!\r\n", conf.verbose)

	// Handle commands
	for {
		// Read command
		readBuf := make([]byte, 4096)
		l, e := (*c).Read(readBuf)
		if e != nil {
			_ = (*c).Close()
			return
		}

		// Log command
		if conf.verbose {
			log.Printf("-> [%s]", strings.Trim(string(readBuf[0:l]), "\r\n "))
		}

		// Add latency
		if conf.latency != 0 {
			time.Sleep(conf.latency * time.Millisecond)
		}

		// Send response
		h, ok := responses[strings.ToUpper(string(readBuf[0:4]))]
		if ok {
			sendResponse(c, h.s, conf.verbose)
			if h.f != nil {
				// Run callback to handle transaction
				h.f(c, readBuf, conf)
			}
		} else {
			sendResponse(c, "500 Command unrecognized\r\n", conf.verbose)
		}
	}
}

func handleData(c *net.Conn, b []byte, conf *config) {
	for {
		// Read data
		l, e := (*c).Read(b)
		if e != nil || l == 0 {
			break
		}

		// Log number of bytes received
		if conf.verbose {
			log.Printf("-- Received %d bytes", l)
		}

		// Check wether we have reached the end
		if bytes.Contains(b, []byte("\r\n.\r\n")) {
			// Add latency
			if conf.latency != 0 {
				time.Sleep(conf.latency)
			}

			// Send response
			sendResponse(c, "250 OK\r\n", conf.verbose)
			break
		}
	}
}

func handleBdat(c *net.Conn, b []byte, conf *config) {
	// TODO Implement BDAT
}

func handleStarttls(c *net.Conn, b []byte, conf *config) {
	*c = tls.Server(*c, &conf.tls)
}

func main() {
	var conf config
	var port, latency, cpus int
	var certFile, keyFile string

	flag.StringVar(&certFile, "cert", "", "Certficate file (PEM encoded)")
	flag.IntVar(&cpus, "cpus", 2, "Number of CPUs/kernel threads used")
	flag.StringVar(&keyFile, "key", "", "Private key file (PEM encoded)")
	flag.IntVar(&latency, "latency", 0, "Latency in milliseconds")
	flag.IntVar(&port, "port", 25, "TCP port")
	flag.BoolVar(&conf.verbose, "verbose", false, "Show the SMTP traffic")

	flag.Parse()

	// Use cpus kernel threads
	runtime.GOMAXPROCS(cpus)

	// Set latency
	if latency < 0 || 1000000 < latency {
		latency = 0
	}
	conf.latency = time.Duration(latency) * time.Millisecond

	if certFile != "" {
		// Load certificate
		if keyFile == "" {
			// Assume the private key is in the same file as the certificate
			keyFile = certFile
		}
		cert, e := tls.LoadX509KeyPair(certFile, keyFile)
		if e != nil {
			// Error!
			log.Panic(e)
			return
		}
		conf.tls.Certificates = []tls.Certificate{cert}
	}

	// Get address:port
	a, e := net.ResolveTCPAddr("tcp4", fmt.Sprintf(":%d", port))
	if e != nil {
		// Error!
		log.Panic(e)
		return
	}

	// Start listening for incoming connections
	l, e := net.ListenTCP("tcp", a)
	if e != nil {
		// Error!
		log.Panic(e)
		return
	}

	// Accept connections then handle each one in a dedicated goroutine
	for {
		c, e := l.Accept()
		if e != nil {
			continue
		}
		go handleConnection(&c, &conf)
	}
}
