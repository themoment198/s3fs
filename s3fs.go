package s3fs

import (
	"context"
	"github.com/minio/minio-go/v7"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type s3fs struct {
	showDirFiles bool
	cli          *minio.Client
}

func NewS3FS(cli *minio.Client, showDirFiles bool) *s3fs {
	return &s3fs{
		showDirFiles: showDirFiles,
		cli:          cli,
	}
}

var emptyListObjectInfo = make([]minio.ObjectInfo, 0)

var defaultStat = &syscall.Stat_t{}

// open path
func (s *s3fs) Open(minioPath string) (http.File, error) {

	parent := filepath.Dir(minioPath)
	if parent == minioPath {
		//
		// it's root, show all bucket list
		//
		buckets, err := s.cli.ListBuckets(context.TODO())
		if err != nil {
			return nil, err
		}
		return s3fsObjPool.Get().(*s3fsObj).withValue(rootType, s, minioPath, true, buckets, nil, nil), nil

	} else if strings.Count(minioPath, "/") == 1 {
		if s.showDirFiles == false {
			return s3fsObjPool.Get().(*s3fsObj).withValue(bucketType, s, minioPath, true, nil, emptyListObjectInfo, nil), nil
		}

		//
		// it's bucket, show sub files
		//
		objs := make([]minio.ObjectInfo, 0)

		bucketName := strings.Split(minioPath, "/")[1]
		objectCh := s.cli.ListObjects(context.TODO(), bucketName, minio.ListObjectsOptions{
			WithVersions: false,
			WithMetadata: false,
			Prefix:       "",
			Recursive:    false,
			MaxKeys:      0,
			StartAfter:   "",
			UseV1:        false,
		})
		for obj := range objectCh {
			if obj.Err != nil {
				return nil, obj.Err
			}
			objs = append(objs, obj)
		}
		return s3fsObjPool.Get().(*s3fsObj).withValue(bucketType, s, minioPath, true, nil, objs, nil), nil

	} else {
		bucketName := strings.Split(minioPath, "/")[1]
		prefix := strings.Join(strings.Split(minioPath, "/")[2:], "/")

		//
		// it's dir, may be is not dir, so we assume it's dir
		//
		objs := make([]minio.ObjectInfo, 0)
		doneCh := make(chan struct{})
		defer close(doneCh)
		objectCh := s.cli.ListObjects(context.TODO(), bucketName, minio.ListObjectsOptions{
			WithVersions: false,
			WithMetadata: false,
			Prefix:       prefix + "/",
			Recursive:    false,
			MaxKeys:      0,
			StartAfter:   "",
			UseV1:        false,
		})
		for obj := range objectCh {
			if obj.Err != nil {
				return nil, obj.Err
			}
			objs = append(objs, obj)
		}
		if len(objs) != 0 {
			if s.showDirFiles == false {
				return s3fsObjPool.Get().(*s3fsObj).withValue(bucketType, s, minioPath, true, nil, emptyListObjectInfo, nil), nil
			}

			return s3fsObjPool.Get().(*s3fsObj).withValue(fileType, s, minioPath, true, nil, objs, nil), nil
		}

		//
		// it's file
		//
		obj, err := s.cli.GetObject(context.TODO(), bucketName, prefix, minio.GetObjectOptions{})
		if err == nil {
			return s3fsObjPool.Get().(*s3fsObj).withValue(fileType, s, minioPath, false, nil, nil, obj), nil
		}
	}

	return nil, io.EOF
}

type minioType int

const (
	rootType   minioType = 0
	bucketType minioType = 1
	fileType   minioType = 2
)

var s3fsObjPool = &sync.Pool{
	New: func() interface{} {
		return &s3fsObj{}
	},
}

type s3fsObj struct {
	tp minioType
	*s3fs
	minioPath string
	isDir     bool

	buckets []minio.BucketInfo
	objects []minio.ObjectInfo

	minioObj *minio.Object

	objectInfoSet []os.FileInfo
}

func (s *s3fsObj) withValue(tp minioType, s3fsObj *s3fs, minioPath string, isDir bool, buckets []minio.BucketInfo, objects []minio.ObjectInfo, minioObj *minio.Object) *s3fsObj {
	s.tp = tp
	s.s3fs = s3fsObj
	s.minioPath = minioPath
	s.isDir = isDir
	s.buckets = buckets
	s.objects = objects
	s.minioObj = minioObj
	s.objectInfoSet = nil
	return s
}

func (s *s3fsObj) Close() error {
	if !s.isDir {
		err := s.minioObj.Close()
		s3fsObjPool.Put(s)
		for _, v := range s.objectInfoSet {
			objectInfoPool.Put(v)
		}
		return err
	}

	s3fsObjPool.Put(s)
	for _, v := range s.objectInfoSet {
		objectInfoPool.Put(v)
	}
	return nil
}

func (s *s3fsObj) Read(p []byte) (int, error) {
	if !s.isDir {
		return s.minioObj.Read(p)
	}
	return 0, nil
}

func (s *s3fsObj) Seek(offset int64, whence int) (int64, error) {
	if !s.isDir {
		return s.minioObj.Seek(offset, whence)
	}
	return 0, nil
}

func (s *s3fsObj) Readdir(_ int) ([]os.FileInfo, error) {
	fileInfos := make([]os.FileInfo, 0)

	if !s.isDir {
		return fileInfos, nil
	}

	switch s.tp {
	case rootType:
		for _, bucket := range s.buckets {
			fileInfos = append(fileInfos, objectInfoPool.Get().(*objectInfo).withValue(true, bucket.Name, 0, bucket.CreationDate))
		}

	case bucketType:
		for _, object := range s.objects {
			var isDir bool
			if strings.HasSuffix(object.Key, "/") {
				isDir = true
			}
			fileInfos = append(fileInfos, objectInfoPool.Get().(*objectInfo).withValue(isDir, filepath.Clean(object.Key), object.Size, object.LastModified))
		}

	case fileType:
		for _, object := range s.objects {
			var isDir bool
			if strings.HasSuffix(object.Key, "/") {
				isDir = true
			}
			fileInfos = append(fileInfos, objectInfoPool.Get().(*objectInfo).withValue(isDir, filepath.Base(object.Key), object.Size, object.LastModified))
		}
	}

	s.objectInfoSet = append(s.objectInfoSet, fileInfos...)
	return fileInfos, nil
}

func (s *s3fsObj) Stat() (os.FileInfo, error) {
	if !s.isDir {
		info, err := s.minioObj.Stat()
		if err != nil {
			return nil, os.ErrNotExist
		}
		fInfo := objectInfoPool.Get().(*objectInfo).withValue(false, s.minioPath, info.Size, info.LastModified)
		s.objectInfoSet = append(s.objectInfoSet, fInfo)
		return fInfo, nil
	}

	fInfo := objectInfoPool.Get().(*objectInfo).withValue(true, s.minioPath, 0, time.Now())
	s.objectInfoSet = append(s.objectInfoSet, fInfo)
	return fInfo, nil
}

var objectInfoPool = &sync.Pool{
	New: func() interface{} {
		return &objectInfo{}
	},
}

type objectInfo struct {
	size  int64
	time  time.Time
	isDir bool
	name  string
}

func (o *objectInfo) withValue(isDir bool, name string, size int64, time time.Time) *objectInfo {
	o.isDir = isDir
	o.name = name
	o.size = size
	o.time = time
	return o
}

func (o *objectInfo) Name() string {
	return o.name
}

func (o *objectInfo) Size() int64 {
	return o.size
}

func (o *objectInfo) Mode() os.FileMode {
	if o.isDir {
		return os.ModeDir | 0600
	}
	return os.FileMode(0600)
}

func (o *objectInfo) ModTime() time.Time {
	return o.time
}

func (o *objectInfo) IsDir() bool {
	return o.isDir
}

func (o *objectInfo) Sys() interface{} {
	return defaultStat
}
