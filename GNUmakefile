default:
	go get ./...
	go get github.com/golang/lint/golint
	go test ./...
	go vet ./...
	golint ./...
.PHONY: default

testacc: clean
	docker version > /dev/null
	go build .
	./mkdeb build mkdeb.json
	mv mkdeb-1.0-amd64.deb docker-testacc/mkdeb-1.0-amd64.deb
	cd docker-testacc && ( docker build --force-rm -t mkdeb-test . | grep -v "Step 3" | grep success )
	docker rmi mkdeb-test > /dev/null
.PHONY: testacc

clean:
	rm -f mkdeb-1.0-amd64.deb docker-testacc/mkdeb-1.0-amd64.deb mkdeb mkdeb.exe
.PHONY: clean

package:
	GOOS=linux GOARCH=amd64 go build .
	mkdeb build mkdeb.json
.PHONY: package
