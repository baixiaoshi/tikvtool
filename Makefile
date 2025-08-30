
build-mac:
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o ./tikvtool

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ./tikvtool

clean:
	rm -rf ./tikvtool