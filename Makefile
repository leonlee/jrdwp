include go.mk

.PHONY: build
build: gomkbuild 

.PHONY: release
release: gomkbuild gomklinux gomkwindows

.PHONY: linux
linux: gomklinux

.PHONY: windows
windows: gomkwindows

.PHONY: xbuild
xbuild: gomkxbuild

.PHONY: clean
clean: gomkclean

.PHONY: run
run: build
				./$(APPBIN)
