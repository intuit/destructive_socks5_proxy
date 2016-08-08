go test -coverprofile cover.txt
go tool cover -html=cover.txt -o cover.html
