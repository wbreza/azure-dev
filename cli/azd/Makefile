PROTO_DIR=grpc/proto
GEN_DIR=pkg/azdext

.PHONY: all proto clean

all: proto

proto:
	# Generate server-side code
	mkdir -p $(GEN_DIR)
	protoc --proto_path=$(PROTO_DIR) \
	       --go_out=$(GEN_DIR) \
	       --go-grpc_out=require_unimplemented_servers=true:$(GEN_DIR) \
	       $(PROTO_DIR)/*.proto
clean:
	rm -rf $(GEN_DIR)
