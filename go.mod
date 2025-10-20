module github.com/stenstromen/dns-over-https

go 1.25.0

// Ignore directories that don't contain Go code
ignore (
	docs/
	examples/
	scripts/
)

require (
	github.com/gorilla/handlers v1.5.2
	github.com/infobloxopen/go-trees v0.0.0-20221216143356-66ceba885ebc
	github.com/miekg/dns v1.1.68
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	golang.org/x/net v0.43.0 // indirect
)

require (
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/redis/go-redis/v9 v9.14.1
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/tools v0.35.0 // indirect
)
