
build: 
	GOOS=linux GOARCH=amd64 go build -o build/linux-amd64/slick-agent
	#GOOS=darwin GOARCH=amd64 go build -o build/mac-amd64/slick-agent
	#GOOS=windows GOARCH=amd64 go build -o build/windows-amd64/slick-agent.exe

clean:
	rm -rf build

