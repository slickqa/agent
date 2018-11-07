
build: 
	GOOS=linux GOARCH=amd64 go build -o build/linux-amd64/slick-agent

clean:
	rm -rf build

deps:
	echo "put go get crap here"
