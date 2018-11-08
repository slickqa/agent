
build: 
	GOOS=linux GOARCH=amd64 go build -o build/linux-amd64/slick-agent

clean:
	rm -rf build

deps:
	go get -u github.com/slickqa/slick
	go get -u github.com/minio/minio-go
	go get -u github.com/namsral/flag
	go get -u gopkg.in/yaml.v2
