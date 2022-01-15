package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"reflect"
	"sync"
	"text/template"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func noRouteHandler(ctx *gin.Context) {
	ctx.JSON(http.StatusNotFound, apiResponse{Error: "Bad Route"})
}

func pingHandler(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, apiResponse{Data: "Pong"})
}

func addHandler(ctx *gin.Context) {
	var (
		dbConn   *sqlx.DB
		dbTx     *sql.Tx
		dbQuery  string
		mux      *sync.Mutex
		item     inventoryRow
		reqBody  apiRequestAddBody
		imgBuff  []byte
		imgMIME  *mimetype.MIME
		imgDir   string
		imgFile  *os.File
		itemHash string
		currUnix time.Time
		currLoc  *time.Location
		err      error
	)

	if dbConn, err = ensureDbMiddleware(ctx); err != nil {
		log.Printf("route: database precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if mux, err = ensureMuxMiddleware(ctx); err != nil {
		log.Printf("route: mutex precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if err = ctx.ShouldBindJSON(&reqBody); err != nil {
		log.Printf("route: malformed request: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Malformed Request",
		})
		return
	}

	imgBuff, err = base64.StdEncoding.DecodeString(reqBody.ImgBase64)
	if err != nil {
		log.Printf("enc: bad base64 image upload: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Bad Base64 Image Encoding",
		})
		return
	}

	imgMIME = mimetype.Detect(imgBuff)
	if !mimetype.EqualsAny(imgMIME.String(), allowedImgMIMETypes...) {
		log.Printf("enc: bad base64 image upload: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Bad Image MIME type",
		})
		return
	}

	if currLoc, err = time.LoadLocation("UTC"); err != nil {
		log.Printf("enc: failed to load time-zone: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Clock Error",
		})
		return
	}

	itemHash = genItemHash()
	currUnix = time.Now().In(currLoc)
	{
		item.ItemID = itemHash
		item.ItemCount = reqBody.ItemCount
		item.ItemPrice = reqBody.ItemPrice
		item.ItemBrand = reqBody.ItemBrand
		item.ItemName = reqBody.ItemName
		item.ItemDesc = reqBody.ItemDesc
		item.CreatedAt = currUnix
		item.UpdatedAt = currUnix
	}

	if dbTx, err = dbConn.Begin(); err != nil {
		log.Printf("db: failed to acquire lock: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	dbQuery = fmt.Sprintf(queryAddItem, dbTable)
	if _, err = dbConn.NamedExec(dbQuery, item); err != nil {
		log.Printf("db: failed to insert row: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if err = dbTx.Commit(); err != nil {
		log.Printf("db: failed to commit transaction: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	imgDir = path.Join(imageRootDir, itemHash)
	if _, err = os.Stat(imgDir); errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(imgDir, os.ModePerm); err != nil {
			log.Printf("fs: failed to create directory: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Image Write Failed",
			})
			return
		}
	}

	mux.Lock()
	imgFile, err = os.Create(path.Join(imgDir, "original"))
	if err != nil {
		log.Printf("fs: failed to open file: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Image Write Failed",
		})
		mux.Unlock()
		return
	}
	defer imgFile.Close()

	if _, err = imgFile.Write(imgBuff); err != nil {
		log.Printf("fs: failed to write image: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Image Write Failed",
		})
		mux.Unlock()
		return

	}
	mux.Unlock()

	ctx.JSON(http.StatusCreated, apiResponse{Data: itemID{itemHash}})
}

func getHandler(ctx *gin.Context) {
	var (
		dbConn  *sqlx.DB
		dbRow   *sqlx.Row
		dbStmt  *sqlx.Stmt
		dbQuery string
		itemURI itemID
		item    inventoryRow
		err     error
	)

	if dbConn, err = ensureDbMiddleware(ctx); err != nil {
		log.Printf("route: database precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if err = ctx.ShouldBindUri(&itemURI); err != nil {
		log.Printf("route: invalid URI: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Invalid URI",
		})
		return
	}

	dbQuery = fmt.Sprintf(queryGetItem, dbTable)
	dbStmt, err = dbConn.Preparex(dbQuery)
	if err != nil {
		log.Printf("db: prepare failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Database Connnection Failure",
		})
		return
	}
	defer dbStmt.Close()

	dbRow = dbStmt.QueryRowx(itemURI.ItemID)
	if err = dbRow.StructScan(&item); err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, apiResponse{
				Error: "No Such Item",
			})
			return
		}

		log.Printf("db: query failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Database Query Failure",
		})
		return
	}

	ctx.JSON(http.StatusOK, apiResponse{
		Data: item,
	})
}

func listHandler(ctx *gin.Context) {
	var (
		dbConn      *sqlx.DB
		dbStmt      *sqlx.Stmt
		dbTmpl      *template.Template
		dbRows      *sqlx.Rows
		dbQuery     string
		dbQueryTmpl bytes.Buffer
		item        inventoryRow
		listQuery   apiRequestListQuery
		err         error

		rows = []inventoryRow{}
	)

	if dbConn, err = ensureDbMiddleware(ctx); err != nil {
		log.Printf("route: database precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if err = ctx.ShouldBindQuery(&listQuery); err != nil {
		log.Printf("route: query string parse failed: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Malformed Request",
		})
		return
	}

	dbQuery = fmt.Sprintf(queryListItems, dbTable)
	dbTmpl, err = template.New("queryListItems").Parse(dbQuery)
	if err != nil {
		log.Printf("db: template parse failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Query Parse Failure",
		})
		return
	}

	if err = dbTmpl.Execute(&dbQueryTmpl, &listQuery); err != nil {
		log.Printf("db: template execute failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Query Parse Failure",
		})
		return
	}

	dbStmt, err = dbConn.Preparex(dbQueryTmpl.String())
	if err != nil {
		log.Printf("db: prepare failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Database Connnection Failure",
		})
		return
	}
	defer dbStmt.Close()

	dbRows, err = dbStmt.Queryx()
	if err != nil {
		log.Printf("db: query failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Database Query Failure",
		})
		return
	}
	defer dbRows.Close()

	for dbRows.Next() {
		err = dbRows.StructScan(&item)
		if err != nil {
			log.Printf("db: failed to fetch row: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Database Query Failure",
			})
			return
		}

		rows = append(rows, item)
	}

	if err = dbRows.Err(); err != nil {
		log.Printf("db: row fetch failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Database Query Failure",
		})
		return
	}

	ctx.JSON(http.StatusOK, apiResponse{Data: rows})
}

func updateHandler(ctx *gin.Context) {
	var (
		itemURI     itemID
		dbConn      *sqlx.DB
		dbTx        *sql.Tx
		dbStmt      *sqlx.Stmt
		mux         *sync.Mutex
		imgBuff     []byte
		imgMIME     *mimetype.MIME
		imgDir      string
		imgFile     *os.File
		dbQuery     string
		err         error
		ok          bool
		upField     apiRequestUpdateQuery
		upValidator apiRequestUpdateBody
		upVal       interface{}
		currUnix    time.Time
		currLoc     *time.Location
	)

	if dbConn, err = ensureDbMiddleware(ctx); err != nil {
		log.Printf("route: database precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if mux, err = ensureMuxMiddleware(ctx); err != nil {
		log.Printf("route: mutex precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if err = ctx.ShouldBindUri(&itemURI); err != nil {
		log.Printf("route: invalid URI: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Invalid URI",
		})
		return
	}

	if err = ctx.ShouldBindQuery(&upField); err != nil {
		log.Printf("route: query string parse failed: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Malformed Request",
		})
		return
	}

	if err = ctx.ShouldBindJSON(&upValidator); err != nil {
		log.Printf("route: malformed request: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Malformed Request",
		})
		return
	}

	if upField.UpdateField != "image_base64" {
		dbQuery = fmt.Sprintf(
			queryUpdateItem, dbTable, upField.UpdateField,
		)

		if dbTx, err = dbConn.Begin(); err != nil {
			log.Printf("db: failed to acquire lock: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Internal Server Error",
			})
			return
		}

		if currLoc, err = time.LoadLocation("UTC"); err != nil {
			log.Printf("enc: failed to load time-zone: %v", err)
			ctx.JSON(http.StatusBadRequest, apiResponse{
				Error: "Clock Error",
			})
			return
		}

		currUnix = time.Now().In(currLoc)

		dbStmt, err = dbConn.Preparex(dbQuery)
		if err != nil {
			log.Printf("db: prepare failed: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Database Connnection Failure",
			})
			dbTx.Rollback()
			return
		}

		upVal, ok = getFieldFromTag(
			"json", upField.UpdateField, upValidator,
		)
		if !ok {
			log.Printf(
				"route: malformed request: %v",
				errors.New("could not find field"),
			)
			ctx.JSON(http.StatusBadRequest, apiResponse{
				Error: "Malformed Request",
			})
			dbTx.Rollback()
			return
		}

		switch reflect.TypeOf(upVal).Kind() {
		case reflect.String:
			_, err = dbStmt.Exec(
				upVal.(string), currUnix, itemURI.ItemID,
			)
		case reflect.Uint64:
			_, err = dbStmt.Exec(
				upVal.(uint64), currUnix, itemURI.ItemID,
			)
		case reflect.Float64:
			log.Println("float")
			_, err = dbStmt.Exec(
				upVal.(float64), currUnix, itemURI.ItemID,
			)
		default:
			log.Printf(
				"route: bad field type: %v",
				errors.New("reflect failed for parsed value"),
			)
			ctx.JSON(http.StatusBadRequest, apiResponse{
				Error: "Bad Field Type",
			})
			dbTx.Rollback()
			return
		}

		if err != nil {
			log.Printf("db: failed to update row: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Internal Server Error",
			})
			dbTx.Rollback()
			return
		}

		if err = dbTx.Commit(); err != nil {
			log.Printf("db: failed to commit transaction: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Internal Server Error",
			})
			dbTx.Rollback()
			return
		}

		ctx.JSON(http.StatusCreated, apiResponse{Data: itemURI})
		return
	}

	imgBuff, err = base64.StdEncoding.DecodeString(upValidator.ImageBase64)
	if err != nil {
		log.Printf("enc: bad base64 image upload: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Bad Base64 Image Encoding",
		})
		return
	}

	imgMIME = mimetype.Detect(imgBuff)
	if !mimetype.EqualsAny(imgMIME.String(), allowedImgMIMETypes...) {
		log.Printf("enc: bad base64 image upload: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Bad Image MIME type",
		})
		return
	}

	mux.Lock()
	imgDir = path.Join(imageRootDir, itemURI.ItemID)
	if _, err = os.Stat(imgDir); err == nil {
		if err = os.RemoveAll(imgDir); err != nil {
			log.Printf("fs: failed to delete directory: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Image Update Failed",
			})
			mux.Unlock()
			return
		}

		if err = os.MkdirAll(imgDir, os.ModePerm); err != nil {
			log.Printf("fs: failed to create directory: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Image Write Failed",
			})
			mux.Unlock()
			return
		}
	}

	imgFile, err = os.Create(path.Join(imgDir, "original"))
	if err != nil {
		log.Printf("fs: failed to open file: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Image Write Failed",
		})
		mux.Unlock()
		return
	}
	defer imgFile.Close()

	if _, err = imgFile.Write(imgBuff); err != nil {
		log.Printf("fs: failed to write image: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Image Write Failed",
		})
		mux.Unlock()
		return
	}
	mux.Unlock()

	ctx.JSON(http.StatusCreated, apiResponse{Data: itemURI})
}

func imgHandler(ctx *gin.Context) {
	var (
		imgURI   itemID
		imgDir   string
		imgThumb imgRequestGetQuery
		imgPath  string
		err      error
	)

	if err = ctx.ShouldBindUri(&imgURI); err != nil {
		log.Printf("route: invalid URI: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Invalid URI",
		})
		return
	}

	if err = ctx.ShouldBindQuery(&imgThumb); err != nil {
		log.Printf("route: query string parse failed: %v", err)
		ctx.Data(http.StatusBadRequest, gin.MIMEPlain, nil)
		return
	}

	imgDir = path.Join(imageRootDir, imgURI.ItemID)
	if _, err = os.Stat(imgDir); errors.Is(err, os.ErrNotExist) {
		log.Printf("route: file not found: %v", err)
		ctx.Data(http.StatusNotFound, gin.MIMEPlain, nil)
		return
	}

	imgPath = path.Join(imgDir, "original")
	if !(imgThumb.Height == 0 && imgThumb.Width == 0) {
		imgPath, err = genImageThumb(
			uint(imgThumb.Height), uint(imgThumb.Width), imgDir,
		)
		if err != nil {
			log.Printf("img: thumbnail generation failed: %v", err)
			ctx.Data(
				http.StatusInternalServerError,
				gin.MIMEPlain, nil,
			)
			return
		}
	}

	ctx.File(imgPath)
}

func deleteHandler(ctx *gin.Context) {
	var (
		dbConn  *sqlx.DB
		dbTx    *sql.Tx
		dbRes   sql.Result
		dbQuery string
		mux     *sync.Mutex
		itemURI itemID
		imgDir  string
		err     error
		tmp     int64
	)

	if dbConn, err = ensureDbMiddleware(ctx); err != nil {
		log.Printf("route: database precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if mux, err = ensureMuxMiddleware(ctx); err != nil {
		log.Printf("route: mutex precondition failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if err = ctx.ShouldBindUri(&itemURI); err != nil {
		log.Printf("route: invalid URI: %v", err)
		ctx.JSON(http.StatusBadRequest, apiResponse{
			Error: "Invalid URI",
		})
		return
	}

	if dbTx, err = dbConn.Begin(); err != nil {
		log.Printf("db: failed to acquire lock: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	dbQuery = fmt.Sprintf(queryDeleteItem, dbTable)
	dbRes, err = dbConn.Exec(dbQuery, itemURI.ItemID)
	if err != nil {
		log.Printf("db: failed to delete row: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if tmp, err = dbRes.RowsAffected(); err != nil {
		log.Printf("db: failed to fetch query result: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	if tmp <= 0 {
		ctx.JSON(http.StatusNotFound, apiResponse{
			Error: "Item Not Found",
		})
		return
	}

	if err = dbTx.Commit(); err != nil {
		log.Printf("db: failed to commit transaction: %v", err)
		ctx.JSON(http.StatusInternalServerError, apiResponse{
			Error: "Internal Server Error",
		})
		return
	}

	mux.Lock()
	imgDir = path.Join(imageRootDir, itemURI.ItemID)
	if _, err = os.Stat(imgDir); err == nil {
		if err = os.RemoveAll(imgDir); err != nil {
			log.Printf("fs: failed to delete directory: %v", err)
			ctx.JSON(http.StatusInternalServerError, apiResponse{
				Error: "Image Delete Failed",
			})
			mux.Unlock()
			return
		}
	}
	mux.Unlock()

	ctx.JSON(http.StatusOK, apiResponse{Data: "OK"})
}
