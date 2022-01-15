package main

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/nfnt/resize"

	_ "github.com/lib/pq"
)

var (
	seed *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func invalidArgs(rnPort, dbPort *int,
	dbHost, dbName, dbUser, dbPass *string) error {

	if *rnPort <= minPortRange || *rnPort > maxPortRange {
		return fmt.Errorf("bad port range for '-port': %d", *rnPort)
	}

	if *dbPort <= minPortRange || *dbPort > maxPortRange {
		return fmt.Errorf("bad port range for '-dbport': %d", *dbPort)
	}

	if len(*dbHost) <= 0 {
		return fmt.Errorf("bad hostname for '-dbhost': %s", *dbHost)
	}

	if len(*dbName) <= 0 {
		return fmt.Errorf("bad database name '-dbname': %s", *dbName)
	}

	if len(*dbUser) <= 0 || len(*dbPass) <= 0 {
		return fmt.Errorf("empty credentials '-dbname' or '-dbpass'")
	}

	return nil
}

func getDBConnStr(dbPort *int, dbHost, dbName, dbUser, dbPass *string) string {
	return fmt.Sprintf(
		dbDSN, dbType, *dbUser, *dbPass, *dbHost, *dbPort, *dbName,
	)
}

func useDbMiddleware(db *sqlx.DB) func(*gin.Context) {
	return func(ctx *gin.Context) {
		ctx.Set(dbSiteKey, db)
		ctx.Next()
	}
}

func useMuxMiddleware(mux *sync.Mutex) func(*gin.Context) {
	return func(ctx *gin.Context) {
		ctx.Set(muxSiteKey, mux)
		ctx.Next()
	}
}

func useNoCacheMiddleware() func(*gin.Context) {
	return func(c *gin.Context) {
		c.Header(
			"Cache-Control",
			"no-cache, no-store, must-revalidate, max-age=0",
		)
		c.Next()
	}
}

func useCORSMiddleware() func(*gin.Context) {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header(
			"Access-Control-Allow-Headers",
			"Content-Type, Content-Length, Accept-Encoding, "+
				"X-CSRF-Token, Authorization, accept, origin, "+
				"Cache-Control, X-Requested-With",
		)
		c.Header(
			"Access-Control-Allow-Methods",
			"GET, POST, PUT, DELETE, OPTIONS",
		)
		c.Next()
	}
}

func ensureDbMiddleware(ctx *gin.Context) (*sqlx.DB, error) {
	var (
		dbConn *sqlx.DB
		ok     bool
	)
	if dbConn, ok = ctx.MustGet(dbSiteKey).(*sqlx.DB); !ok {
		return nil, fmt.Errorf("router: database undefined in context")
	}

	return dbConn, nil
}

func ensureMuxMiddleware(ctx *gin.Context) (*sync.Mutex, error) {
	var (
		mux *sync.Mutex
		ok  bool
	)
	if mux, ok = ctx.MustGet(muxSiteKey).(*sync.Mutex); !ok {
		return nil, fmt.Errorf("router: mutex undefined in context")
	}

	return mux, nil
}

func genItemHash() string {
	var buff []byte = make([]byte, hashStrSize)

	for i := range buff {
		buff[i] = hashCharSet[seed.Intn(len(hashCharSet))]
	}

	return string(buff)
}

func resizePng(height, width uint, in *os.File, out *os.File) error {
	var (
		inBuff  image.Image
		outBuff image.Image
		err     error
	)

	if inBuff, err = png.Decode(in); err != nil {
		return err
	}

	outBuff = resize.Thumbnail(width, height, inBuff, resize.Lanczos3)

	if err = png.Encode(out, outBuff); err != nil {
		return err
	}

	return nil
}

func resizeJpg(height, width uint, in *os.File, out *os.File) error {
	var (
		inBuff  image.Image
		outBuff image.Image
		err     error
	)

	if inBuff, err = jpeg.Decode(in); err != nil {
		return err
	}

	outBuff = resize.Thumbnail(width, height, inBuff, resize.Lanczos3)

	if err = jpeg.Encode(out, outBuff, nil); err != nil {
		return err
	}

	return nil
}

func genImageThumb(height, width uint, imgDir string) (string, error) {
	var (
		in      *os.File
		out     *os.File
		mtype   *mimetype.MIME
		inPath  string
		outPath string
		err     error
	)

	inPath = path.Join(imgDir, "original")
	if mtype, err = mimetype.DetectFile(inPath); err != nil {
		return "", err
	}

	if in, err = os.Open(inPath); err != nil {
		return "", err
	}
	defer in.Close()

	outPath = path.Join(imgDir, fmt.Sprintf("thumb_%dx%d", height, width))
	if _, err = os.Stat(outPath); err == nil {
		return outPath, nil
	}

	if out, err = os.Create(outPath); err != nil {
		return "", err
	}
	defer out.Close()

	switch mtype.Extension() {
	case ".png":
		if err = resizePng(height, width, in, out); err != nil {
			return "", err
		}
	case ".jpg", ".jpeg":
		if err = resizeJpg(height, width, in, out); err != nil {
			return "", err
		}
	default:
		return "", errors.New("bad image format")
	}

	return outPath, nil
}

func contains(haystack []string, needle string) bool {
	for _, str := range haystack {
		if str == needle {
			return true
		}
	}

	return false
}

func getFieldFromTag(tag, val string, data interface{}) (interface{}, bool) {
	var (
		sVal   reflect.Value
		sType  reflect.Type
		sField reflect.StructField
		sTag   string
		ok     bool
		r      interface{}
	)

	sVal = reflect.ValueOf(data)
	sType = reflect.TypeOf(data)

	for i := 0; i < sType.NumField(); i++ {
		sField = sType.Field(i)
		if sTag, ok = sField.Tag.Lookup(tag); ok {
			if len(sTag) > 0 && sTag == val {
				r = reflect.Indirect(sVal).Field(i).Interface()
				return r, true
			}
		}
	}

	return nil, false
}
