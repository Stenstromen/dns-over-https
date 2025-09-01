package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	redisClient *redis.Client
)

func InitRedis() {
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:        redisURL,
			DialTimeout: 5 * time.Second,
			Network:     "tcp",
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := redisClient.Ping(ctx).Result()
		if err != nil {
			log.Printf("Failed to connect to Redis: %v", err)
			redisClient = nil
		} else {
			log.Printf("Successfully connected to Redis at %s", redisURL)
		}
	} else {
		log.Printf("No REDIS_URL provided, running without cache")
	}
}

func main() {
	var (
		configPath string
		conf       *config
		err        error
	)

	flag.StringVar(&configPath, "conf", "", "configuration file path")
	flag.Parse()

	log.Printf("Starting with GOMAXPROCS=%d (container-aware)", runtime.GOMAXPROCS(0))

	conf = &config{
		Listen:   []string{"0.0.0.0:8053"},
		Path:     "/dns-query",
		Upstream: []string{"udp:8.8.8.8:53"},
		Timeout:  10,
		Tries:    3,
		Verbose:  false,
	}

	// Override with environment variables if present
	if listen := os.Getenv("DOH_SERVER_LISTEN"); listen != "" {
		// Ensure proper address format
		if !strings.Contains(listen, ":") {
			listen = "0.0.0.0:" + listen
		}
		conf.Listen = []string{listen}
	}

	if prefix := os.Getenv("DOH_HTTP_PREFIX"); prefix != "" {
		conf.Path = prefix
	}

	if upstream := os.Getenv("DOH_UPSTREAM_DNS"); upstream != "" {
		conf.Upstream = strings.Split(upstream, ",")
	}

	if timeout := os.Getenv("DOH_SERVER_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			conf.Timeout = uint(t)
		}
	}

	if tries := os.Getenv("DOH_SERVER_TRIES"); tries != "" {
		if t, err := strconv.Atoi(tries); err == nil {
			conf.Tries = uint(t)
		}
	}

	if verbose := os.Getenv("DOH_SERVER_VERBOSE"); verbose != "" {
		conf.Verbose = verbose == "true"
	}

	// Initialize Redis
	InitRedis()

	// Create server
	server, err := NewServer(conf)
	if err != nil {
		log.Fatalln(err)
	}

	if conf.Verbose {
		log.Printf("Configuration: %+v", conf)
	}

	server.Start()
}
