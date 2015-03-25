echo "Building and starting master"
goimports -w . && go build master.go && ./master -v=5 -logtostderr -build_log_base_dir="./logs" -config="./build.conf"
echo "Building and starting slave #1 (:50051)"
go build slave.go && ./slave -address=":50051" -v=5 -logtostderr
echo "Starting slave #2 (:50052)
./slave -address=":50052" -v=5 -logtostderr
