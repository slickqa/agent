
build: 
	GOOS=linux GOARCH=amd64 go build -o build/linux-amd64/slick-agent
	GOOS=darwin GOARCH=amd64 go build -o build/mac-amd64/slick-agent
	GOOS=windows GOARCH=amd64 go build -o build/windows-amd64/slick-agent.exe

clean:
	rm -rf build

deps:
	go get -u github.com/slickqa/slick
	go get -u github.com/namsral/flag
	go get -u gopkg.in/yaml.v2
	go get -u github.com/vova616/screenshot
	go get -u github.com/BurntSushi/xgb
