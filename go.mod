module github.com/stenstromen/dns-over-https

go 1.26.0

// Ignore directories that don't contain Go code
ignore (
	docs/
	examples/
	scripts/
)

require (
	github.com/gorilla/handlers v1.5.2
	github.com/infobloxopen/go-trees v0.0.0-20221216143356-66ceba885ebc
	github.com/miekg/dns v1.1.73
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/net v0.57.0 // indirect
)

require (
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/redis/go-redis/v9 v9.22.0
	golang.org/x/sys v0.47.0 // indirect
)
