package main

import (
	"flag"
	"fmt"
	"log"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	_ "github.com/lib/pq"
)

func main() {
	var (
		// For flags.
		rnPort = flag.Int("port", defaultRnPort, "server port")
		dbHost = flag.String("dbhost", defaultDbHost, "database host")
		dbPort = flag.Int("dbport", defaultDbPort, "database port")
		dbName = flag.String("dbname", defaultDbName, "database name")
		dbUser = flag.String("dbuser", "", "database username")
		dbPass = flag.String("dbpass", "", "database password")
		dbgLog = flag.Bool("debug", false, "debug logging")

		err error

		dbConnStr string
		db        *sqlx.DB

		mux sync.Mutex

		router *gin.Engine
		api    *gin.RouterGroup
		img    *gin.RouterGroup
	)

	// Parse and validate flags.
	flag.Parse()

	err = invalidArgs(rnPort, dbPort, dbHost, dbName, dbUser, dbPass)
	if err != nil {
		log.Fatalf("arg: invalid command-line arguments: %v", err)
	}

	// Setup a connection to the database.
	dbConnStr = getDBConnStr(dbPort, dbHost, dbName, dbUser, dbPass)
	if len(dbConnStr) <= 0 {
		log.Fatalf("db: failed to construct connection string")
	}

	if db, err = sqlx.Open(dbType, dbConnStr); err != nil {
		log.Fatalf("db: failed to open connection: %v", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatalf("db: database pinf failed %v", err)
	}

	// Setup the router.
	if !*dbgLog {
		gin.SetMode(gin.ReleaseMode)
	}

	router = gin.Default()
	router.Use(useDbMiddleware(db))
	router.Use(useMuxMiddleware(&mux))
	router.Use(useCORSMiddleware())
	router.Use(useNoCacheMiddleware())
	router.NoRoute(noRouteHandler)

	api = router.Group("/api")
	{
		api.GET("/ping", pingHandler)
		api.POST("/add", addHandler)
		api.OPTIONS("/add", pingHandler)
		api.GET("/get/:item_id", getHandler)
		api.GET("/list", listHandler)
		api.PUT("/update/:item_id", updateHandler)
		api.OPTIONS("/update/:item_id", pingHandler)
		api.DELETE("/delete/:item_id", deleteHandler)
		api.OPTIONS("/delete/:item_id", pingHandler)
	}

	img = router.Group("/img")
	{
		img.GET("/:item_id", imgHandler)
	}

	router.Run(fmt.Sprintf("%s:%d", defaultRnHost, *rnPort))
}
