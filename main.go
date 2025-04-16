package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
)

type RateLimiter struct {
	client  *redis.Client
	limit   int
	window  time.Duration
	context context.Context
}

func NewRateLimiter(client *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		client:  client,
		limit:   limit,
		window:  window,
		context: context.Background(),
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	pipe := rl.client.TxPipeline()
	pipe.Incr(rl.context, key)

	incr := pipe.Incr(rl.context, key)
	pipe.Expire(rl.context, key, rl.window)

	_, err := pipe.Exec(rl.context)
	if err != nil {
		log.Print(err)
		return false
	}

	return incr.Val() <= int64(rl.limit)
}

func rateLimiterMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !rl.Allow(clientIP) {
			log.Println("Rate limit exceeded")
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	rateLimiter := NewRateLimiter(client, 5, 1*time.Minute)

	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, world!")
	})

	handler := rateLimiterMiddleware(rateLimiter, router)

	http.ListenAndServe(":8080", handler)
}
