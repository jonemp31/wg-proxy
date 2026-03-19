.PHONY: build install clean

build:
	go build -o bin/wg-manager ./cmd/wg-manager/

install: build
	install -m 755 bin/wg-manager /usr/local/bin/wg-manager

clean:
	rm -rf bin/
