# s3fs
s3 implementation of golang interface http.FileSystem

### demo

![demos](https://raw.githubusercontent.com/themoment198/s3fs/main/demos.png)

### Usage

##### Install minio from docker

```
docker pull minio/minio
docker run -p 9000:9000 --name minio1 -e "MINIO_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE" -e "MINIO_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"  -d minio/minio server /data
```

##### test code

```
package main

import (
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/themoment198/s3fs"
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


```

##### bucket and key
 
Take 'http://localhost:3000/static/bucket2/sub1/tmp1.txt' as an example. 'bucket2' is the name of the bucket, and 'sub1/tmp1.txt' is the key of the file in the bucket.

Access 'http://localhost:3000/static/' get all the buckets

### Implementation Details

Many implementations is inspired by https://github.com/harshavardhana/s3www, thanks for harshavardhana's good work!
