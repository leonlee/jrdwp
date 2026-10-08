APP := jrdwp
PLATFORMS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64 windows/386

.PHONY: build release linux windows test clean run

build:
	go build -o $(APP)

# release builds every platform into dist/ as jrdwp_<os>_<arch>[.exe].
release:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		out=dist/$(APP)_$${os}_$${arch}$$ext; echo $$out; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -o $$out || exit 1; \
	done

linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(APP).bin

windows:
	GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -o $(APP).exe

test:
	go test -race ./...

clean:
	rm -rf $(APP) $(APP).bin $(APP).exe dist

run: build
	./$(APP)
