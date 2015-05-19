package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

type handler struct {
	s string
	f func(net.Conn, []byte, time.Duration, bool)
}

var verbose bool

var responses = map[string]handler{
	"EHLO": {"250-Pleased to meet you!\r\n250-PIPELINING\r\n250 CHUNKING\r\n", nil},
	"HELO": {"250 Pleased to meet you!\r\n", nil},
	"MAIL": {"250 OK\r\n", nil},
	"RCPT": {"250 OK\r\n", nil},
	"DATA": {"354 End data with <CR><LF>.<CR><LF>\r\n", handleData}, // Need to read data until \r\n.\r\n is received.
	"BDAT": {"250 OK\r\n", handleBdat},                              // Should be sent once the data has been reveived
	"RSET": {"250 OK\r\n", nil},
	"QUIT": {"221 Goodbye\r\n", nil}}

func sendResponse(c net.Conn, s string, verbose bool) {
			c.Write([]byte(s))
			if verbose {
				log.Printf("<- %s", s)
			}
}

func handleConnection(c net.Conn, latency time.Duration, verbose bool) {
	// Print banner
	sendResponse(c, "220 Welcome to Blackhole SMTP!\r\n", verbose)
	for {
		readBuf := make([]byte, 4096)
		l, e := c.Read(readBuf)
		if e != nil {
			_ = c.Close()
			return
		}
		time.Sleep(latency * time.Millisecond)
		if verbose {
			log.Printf("-> [%s]", strings.Trim(string(readBuf[0:l]), "\r\n "))
		}
		h, ok := responses[string(readBuf[0:4])]
		if ok {
			sendResponse(c, h.s, verbose)
			if h.f != nil {
				h.f(c, readBuf, latency, verbose)
			}
		} else {
			sendResponse(c, "500 Command unrecognized\r\n", verbose)
		}
	}
}

func handleData(c net.Conn, b []byte, latency time.Duration, verbose bool) {
	for {
		l, e := c.Read(b)
		if e != nil || l == 0 {
			break
		}
		if bytes.Contains(b, []byte("\r\n.\r\n")) {
			time.Sleep(latency * time.Millisecond)
			sendResponse(c, "250 OK\r\n", verbose)
			break
		}
	}
}

func handleBdat(c net.Conn, b []byte, latency time.Duration, verbose bool) {
}

func main() {
	var port int
	var latency int
	var verbose bool

	flag.IntVar(&port, "port", 25, "TCP port")
	flag.IntVar(&latency, "latency", 0, "Latency in milliseconds")
	flag.BoolVar(&verbose, "verbose", false, "Show the SMTP traffic")

	flag.Parse()

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
		go handleConnection(c, time.Duration(latency), verbose)
	}
}
