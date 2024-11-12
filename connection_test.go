// Copyright (c) 2014-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by the MIT license.
// See LICENSE file for details.
package balancers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestHttpConnection(t *testing.T) {
	var visited bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited = true
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	conn := NewHttpConnection(url, http.DefaultClient)
	if conn == nil {
		t.Fatal("expected connection")
	}

	// 等待心跳检查完成
	time.Sleep(100 * time.Millisecond)

	if !visited {
		t.Error("expected server to be visited")
	}
	if conn.URL() != url {
		t.Errorf("expected URL %v; got: %v", url, conn.URL())
	}
	if conn.IsBroken() {
		t.Error("expected connection to not be broken")
	}
}

func TestHttpConnectionReturningInternalServerErrorIsBroken(t *testing.T) {
	var visited bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	conn := NewHttpConnection(url, http.DefaultClient)
	if conn == nil {
		t.Fatal("expected connection")
	}

	// 等待心跳检查完成
	time.Sleep(100 * time.Millisecond)

	if !visited {
		t.Error("expected server to be visited")
	}
	if !conn.IsBroken() {
		t.Error("expected connection to be broken")
	}
}

func TestHttpConnectionToNonexistentServer(t *testing.T) {
	url, _ := url.Parse("http://localhost:12345")
	conn := NewHttpConnection(url, http.DefaultClient)
	if conn == nil {
		t.Fatal("expected connection")
	}
	if conn.URL() != url {
		t.Errorf("expected URL %v; got: %v", url, conn.URL())
	}
	broken := conn.IsBroken()
	if !broken {
		t.Error("expected connection to be broken")
	}
}

func TestHttpConnectionExponentialBackoff(t *testing.T) {
	// 启用测试模式
	SetTestMode(true)
	defer SetTestMode(false) // 测试结束后恢复

	var requestTimes []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	conn := NewHttpConnection(url, http.DefaultClient)

	// 等待足够长的时间以观察多次重试
	time.Sleep(700 * time.Millisecond) // 等待时间缩短到毫秒级别

	if len(requestTimes) < 2 {
		t.Fatalf("expected at least 2 retry attempts, got %d", len(requestTimes))
	}

	// 验证第二次重试间隔应该是 200ms
	firstInterval := requestTimes[1].Sub(requestTimes[0])
	if firstInterval < 190*time.Millisecond || firstInterval > 250*time.Millisecond {
		t.Errorf("first retry interval should be around 200 milliseconds, got %v", firstInterval)
	}

	// 如果有第二次重试，验证间隔是否翻倍
	if len(requestTimes) > 2 {
		secondInterval := requestTimes[2].Sub(requestTimes[1])
		if secondInterval < 390*time.Millisecond || secondInterval > 450*time.Millisecond {
			t.Errorf("second retry interval should be around 400 milliseconds, got %v", secondInterval)
		}
	}

	conn.Close()
}
