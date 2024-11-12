// Copyright (c) 2014-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by the MIT license.
// See LICENSE file for details.
package balancers

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Connection is a single connection to a host. It is defined by a URL.
// It also maintains state in the form that a connection can be broken.
// TODO(oe) Not sure if this abstraction is necessary.
type Connection interface {
	// URL to the host.
	URL() *url.URL
	// IsBroken must return true if the connection to URL is currently not available.
	IsBroken() bool
}

// HttpConnection is a HTTP connection to a host.
// It implements the Connection interface and can be used by balancer
// implementations.
type HttpConnection struct {
	sync.Mutex
	url                  *url.URL
	broken               bool
	heartbeatStop        chan bool
	client               *http.Client
	logger               *log.Logger
	userAgent            string
	currentRetryInterval time.Duration
}

const (
	// 生产环境的时间间隔
	initialRetryInterval = 30 * time.Second
	maxRetryInterval     = 5 * time.Minute
	retryMultiplier      = 2

	// 测试环境的时间间隔
	testInitialRetryInterval = 100 * time.Millisecond
	testMaxRetryInterval     = 500 * time.Millisecond
)

var testMode bool = false

// 添加测试辅助函数
func SetTestMode(enabled bool) {
	testMode = enabled
}

// NewHttpConnection creates a new HTTP connection to the given URL.
func NewHttpConnection(url *url.URL, client *http.Client) *HttpConnection {
	c := &HttpConnection{
		url:                  url,
		heartbeatStop:        make(chan bool),
		client:               client,
		logger:               log.New(os.Stderr, "", log.LstdFlags),
		userAgent:            os.Getenv("USER_AGENT"),
		currentRetryInterval: initialRetryInterval,
	}

	if testMode {
		c.currentRetryInterval = testInitialRetryInterval
	}
	c.checkBroken()
	go c.heartbeat()
	return c
}

// Close this connection.
func (c *HttpConnection) Close() error {
	c.Lock()
	defer c.Unlock()
	c.heartbeatStop <- true // wait for heartbeat ticker to stop
	c.broken = false
	return nil
}

// heartbeat periodically checks if the connection is broken.
func (c *HttpConnection) heartbeat() {
	for {
		select {
		case <-time.After(c.getNextInterval()):
			c.checkBroken()
		case <-c.heartbeatStop:
			return
		}
	}
}

// getNextInterval returns the next interval for the heartbeat.
func (c *HttpConnection) getNextInterval() time.Duration {
	c.Lock()
	defer c.Unlock()

	if testMode {
		if !c.broken {
			c.currentRetryInterval = testInitialRetryInterval
			return testInitialRetryInterval
		}

		nextInterval := c.currentRetryInterval * retryMultiplier
		if nextInterval > testMaxRetryInterval {
			nextInterval = testMaxRetryInterval
		}
		c.currentRetryInterval = nextInterval
		if c.broken {
			c.logger.Printf("Connection broken, will retry in %v", nextInterval)
		}
		return nextInterval
	}

	if !c.broken {
		c.currentRetryInterval = initialRetryInterval
		return initialRetryInterval
	}

	nextInterval := c.currentRetryInterval * retryMultiplier
	if nextInterval > maxRetryInterval {
		nextInterval = maxRetryInterval
	}
	c.currentRetryInterval = nextInterval
	c.logger.Printf("Connection broken, will retry in %v", nextInterval)
	return nextInterval
}

// checkBroken checks if the HTTP connection is alive.
func (c *HttpConnection) checkBroken() {
	c.Lock()
	defer c.Unlock()

	req, err := http.NewRequest(http.MethodOptions, c.url.String(), strings.NewReader(""))
	if err != nil {
		c.broken = true
		c.logger.Printf("Failed to create request for %s: %s", c.url.String(), err.Error())
		return
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	// Use a standard HTTP client with a timeout of 5 seconds.
	res, err := c.client.Do(req)
	if err == nil {
		defer res.Body.Close()
		body, _ := ioutil.ReadAll(res.Body)
		if res.StatusCode == http.StatusOK {
			c.broken = false
		} else {
			c.broken = true
			c.logger.Printf("Request to %s failed with status %d: %s", c.url.String(), res.StatusCode, string(body))
		}
	} else {
		c.broken = true
		c.logger.Printf("Request to %s failed: %s", c.url.String(), err.Error())
	}
}

// URL returns the URL of the HTTP connection.
func (c *HttpConnection) URL() *url.URL {
	return c.url
}

// IsBroken returns true if the HTTP connection is currently broken.
func (c *HttpConnection) IsBroken() bool {
	return c.broken
}
