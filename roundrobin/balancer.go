// Copyright (c) 2014-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by the MIT license.
// See LICENSE file for details.
package roundrobin

import (
	"net/url"
	"sync"
	"time"

	"net/http"

	"github.com/tianlin/balancers"
)

// BalancerOptions 包含负载均衡器的配置选项
type BalancerOptions struct {
	client         *http.Client
	connectTimeout time.Duration
	retryTimeout   time.Duration
}

// Option 定义配置选项的函数类型
type Option func(*BalancerOptions)

// WithClient 设置 HTTP 客户端
func WithClient(client *http.Client) Option {
	return func(o *BalancerOptions) {
		o.client = client
	}
}

// WithConnectTimeout 设置连接超时时间
func WithConnectTimeout(timeout time.Duration) Option {
	return func(o *BalancerOptions) {
		o.connectTimeout = timeout
	}
}

// WithRetryTimeout 设置重试超时时间
func WithRetryTimeout(timeout time.Duration) Option {
	return func(o *BalancerOptions) {
		o.retryTimeout = timeout
	}
}

// 默认选项
var defaultOptions = BalancerOptions{
	client:         http.DefaultClient,
	connectTimeout: 30 * time.Second,
	retryTimeout:   5 * time.Minute,
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
	// 使用默认选项
	options := defaultOptions

	// 应用自定义选项
	for _, opt := range opts {
		opt(&options)
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
			options.connectTimeout,
			options.retryTimeout,
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
