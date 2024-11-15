// Copyright (c) 2014-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by the MIT license.
// See LICENSE file for details.
package roundrobin

import (
	"errors"
	"net/url"
	"sync"
	"time"

	"net/http"

	"github.com/tianlin/balancers"
)

// BalancerOptions 包含负载均衡器的配置选项
type BalancerOptions struct {
	client               *http.Client
	initialRetryInterval time.Duration
	maxRetryInterval     time.Duration
}

// Option 定义配置选项的函数类型
type Option func(*BalancerOptions)

// WithClient 设置 HTTP 客户端
func WithClient(client *http.Client) Option {
	return func(o *BalancerOptions) {
		o.client = client
	}
}

// WithInitialRetryInterval 设置初始重试间隔时间
func WithInitialRetryInterval(interval time.Duration) Option {
	return func(o *BalancerOptions) {
		o.initialRetryInterval = interval
	}
}

// WithMaxRetryInterval 设置最大重试间隔时间
func WithMaxRetryInterval(interval time.Duration) Option {
	return func(o *BalancerOptions) {
		o.maxRetryInterval = interval
	}
}

// 默认选项
var defaultOptions = BalancerOptions{
	client:               http.DefaultClient,
	initialRetryInterval: 30 * time.Second,
	maxRetryInterval:     5 * time.Minute,
}

// Balancer implements a round-robin balancer.
type Balancer struct {
	sync.Mutex // guards the following variables
	conns      []balancers.Connection
	idx        int // index into conns
}

// NewBalancer creates a new round-robin balancer. It can be initializes by
// a variable number of connections. To use plain URLs instead of
// connections, use NewBalancerFromURL.
func NewBalancer(conns ...balancers.Connection) (balancers.Balancer, error) {
	b := &Balancer{
		conns: make([]balancers.Connection, 0),
	}
	if len(conns) > 0 {
		b.conns = append(b.conns, conns...)
	}
	return b, nil
}

// NewBalancerFromURL 使用 Option 模式重构
func NewBalancerFromURL(urls []string, opts ...Option) (*Balancer, error) {
	options := defaultOptions

	for _, opt := range opts {
		opt(&options)
	}

	// 检查重试间隔配置的合法性
	if options.initialRetryInterval <= 0 {
		return nil, errors.New("initial retry interval must be greater than 0")
	}
	if options.maxRetryInterval <= 0 {
		return nil, errors.New("max retry interval must be greater than 0")
	}
	if options.maxRetryInterval < options.initialRetryInterval {
		return nil, errors.New("max retry interval must be greater than or equal to initial retry interval")
	}

	b := &Balancer{
		conns: make([]balancers.Connection, 0),
	}

	for _, rawurl := range urls {
		u, err := url.Parse(rawurl)
		if err != nil {
			return nil, err
		}
		b.conns = append(b.conns, balancers.NewHttpConnection(
			u,
			options.client,
			options.initialRetryInterval,
			options.maxRetryInterval,
		))
	}
	return b, nil
}

// Get returns a connection from the balancer that can be used for the next request.
// ErrNoConn is returns when no connection is available.
func (b *Balancer) Get() (balancers.Connection, error) {
	b.Lock()
	defer b.Unlock()

	if len(b.conns) == 0 {
		return nil, balancers.ErrNoConn
	}

	var conn balancers.Connection
	for i := 0; i < len(b.conns); i++ {
		candidate := b.conns[b.idx]
		b.idx = (b.idx + 1) % len(b.conns)
		if !candidate.IsBroken() {
			conn = candidate
			break
		}
	}

	if conn == nil {
		return nil, balancers.ErrNoConn
	}
	return conn, nil
}

// Connections returns a list of all connections.
func (b *Balancer) Connections() []balancers.Connection {
	b.Lock()
	defer b.Unlock()
	conns := make([]balancers.Connection, len(b.conns))
	for i, c := range b.conns {
		if oc, ok := c.(*balancers.HttpConnection); ok {
			// Make a clone
			cr := new(balancers.HttpConnection)
			*cr = *oc
			conns[i] = cr
		}
	}
	return conns
}
