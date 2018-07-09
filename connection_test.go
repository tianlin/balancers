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
	if conn.URL() != url {
		t.Errorf("expected URL %v; got: %v", url, conn.URL())
	}
	broken := conn.IsBroken()
	if broken {
		t.Error("expected connection to not be broken")
	}
}

func TestHttpConnectionReturningInternalServerErrorIsBroken(t *testing.T) {
	var visited bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	server.Close()

	url, _ := url.Parse(server.URL)
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

func TestHttpConnectionHeartbeat(t *testing.T) {
	var count int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count += 1
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	conn := NewHttpConnection(url, http.DefaultClient).HeartbeatDuration(2 * time.Second)
	if conn == nil {
		t.Fatal("expected connection")
	}
	if conn.URL() != url {
		t.Errorf("expected URL %v; got: %v", url, conn.URL())
	}
	time.Sleep(3 * time.Second)
	err := conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 { // 1 on NewConnection + 1 for a heartbeat
		t.Errorf("expected %d heartbeats; got: %d", 2, count)
	}
}
