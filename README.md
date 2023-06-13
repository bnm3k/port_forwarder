# Port Forwarder

Forwards tcp connections to a given address.

## Build

```
go build -o port-forwarder main.go
```

## Usage

```
Usage of ./port-forwarder:
  -f, --forward-to string   Address to forward incoming connections
  -l, --listen-at string    Address to listen at incoming connections (default "localhost:8000")
  -v, --verbose             Verbose (default true)
```
