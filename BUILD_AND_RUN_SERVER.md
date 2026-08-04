cd /home/polin/git/NFC-verification/tag-reader-server

# Build the binary
go build -o sourceserver-local .

# Run without HTTPS on default port 4430
./sourceserver-local

# Or with debug mode (uses test key for NFC verification)
./sourceserver-local --debug

# Or specify a different port
./sourceserver-local --port 8080