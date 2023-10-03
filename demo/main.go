package main

import (
	"github.com/themoment198/s3fs"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"log"
	"net/http"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
}

func main() {
	cli, err := minio.New("localhost:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", ""),
		Secure: false,
	})
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc(
		"/static/",
		http.StripPrefix("/static/", http.FileServer(s3fs.NewS3FS(cli, true))).ServeHTTP,
	)
	err = http.ListenAndServe(":3000", nil)
	if err != nil {
		log.Fatal(err)
	}
}
