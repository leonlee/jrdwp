APP := jrdwp

.PHONY: build release linux windows test clean run

build:
	go build -o $(APP)

release: build linux windows

linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(APP).bin

windows:
	GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -o $(APP).exe

test:
	go test -race ./...

clean:
	rm -f $(APP) $(APP).bin $(APP).exe

run: build
	./$(APP)
