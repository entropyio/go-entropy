# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: entropy android ios entropy-cross swarm evm all test clean
.PHONY: entropy-linux entropy-linux-386 entropy-linux-amd64 entropy-linux-mips64 entropy-linux-mips64le
.PHONY: entropy-linux-arm entropy-linux-arm-5 entropy-linux-arm-6 entropy-linux-arm-7 entropy-linux-arm64
.PHONY: entropy-darwin entropy-darwin-386 entropy-darwin-amd64
.PHONY: entropy-windows entropy-windows-386 entropy-windows-amd64

GOBIN = $(shell pwd)/build/bin
GO ?= latest

entropy:
	build/env.sh go run build/ci.go install ./cmd/entropy
	@echo "Done building."
	@echo "Run \"$(GOBIN)/entropy\" to launch entropy."

swarm:
	build/env.sh go run build/ci.go install ./cmd/swarm
	@echo "Done building."
	@echo "Run \"$(GOBIN)/swarm\" to launch swarm."

all:
	build/env.sh go run build/ci.go install

android:
	build/env.sh go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/entropy.aar\" to use the library."

ios:
	build/env.sh go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/Entropy.framework\" to use the library."

test: all
	build/env.sh go run build/ci.go test

lint: ## Run linters.
	build/env.sh go run build/ci.go lint

clean:
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/kevinburke/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go get -u github.com/golang/protobuf/protoc-gen-go
	env GOBIN= go install ./cmd/abigen
	@type "npm" 2> /dev/null || echo 'Please install node.js and npm'
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

# Cross Compilation Targets (xgo)

entropy-cross: entropy-linux entropy-darwin entropy-windows entropy-android entropy-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/entropy-*

entropy-linux: entropy-linux-386 entropy-linux-amd64 entropy-linux-arm entropy-linux-mips64 entropy-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-*

entropy-linux-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/entropy
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep 386

entropy-linux-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/entropy
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep amd64

entropy-linux-arm: entropy-linux-arm-5 entropy-linux-arm-6 entropy-linux-arm-7 entropy-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep arm

entropy-linux-arm-5:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/entropy
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep arm-5

entropy-linux-arm-6:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/entropy
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep arm-6

entropy-linux-arm-7:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v ./cmd/entropy
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep arm-7

entropy-linux-arm64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/entropy
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep arm64

entropy-linux-mips:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/entropy
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep mips

entropy-linux-mipsle:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/entropy
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep mipsle

entropy-linux-mips64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/entropy
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep mips64

entropy-linux-mips64le:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/entropy
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/entropy-linux-* | grep mips64le

entropy-darwin: entropy-darwin-386 entropy-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/entropy-darwin-*

entropy-darwin-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/entropy
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-darwin-* | grep 386

entropy-darwin-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/entropy
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-darwin-* | grep amd64

entropy-windows: entropy-windows-386 entropy-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/entropy-windows-*

entropy-windows-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/entropy
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-windows-* | grep 386

entropy-windows-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/entropy
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/entropy-windows-* | grep amd64
