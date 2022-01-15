package main

const (
	defaultRnHost string = "0.0.0.0"
	defaultRnPort int    = 8080
	defaultDbPort int    = 5432
	defaultDbHost string = "db"
	defaultDbName string = "shopify"

	maxPortRange int = 65535
	minPortRange int = 0

	dbType    string = "postgres"
	dbSiteKey string = "shopify-db"
	dbDSN     string = "%s://%s:%s@%s:%d/%s?sslmode=disable"
	dbTable   string = "inventory"

	hashStrSize int    = 8
	hashCharSet string = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"0123456789"

	maxImageThumbPx uint = 8192

	muxSiteKey string = "imgFsMux"
)

var (
	imageRootDir = "/tmp/shopify-pe"

	allowedImgMIMETypes = []string{
		"image/png",
		"image/jpeg",
	}

	queryAddItem string = "INSERT INTO %s (item_id, created_at, " +
		"updated_at, item_count, item_price, item_brand, item_name, " +
		"item_desc) values (:item_id, :created_at, :updated_at, " +
		":item_count, :item_price, :item_brand, :item_name, " +
		":item_desc)"

	queryGetItem string = "SELECT * from %s where item_id = $1 LIMIT 1"

	queryListItems string = "SELECT * FROM %s ORDER BY {{ .OrderBy }} " +
		"{{ .Order }}"

	queryUpdateItem string = "UPDATE %s SET %s = $1, updated_at = $2 " +
		"WHERE item_id = $3"

	queryDeleteItem string = "DELETE from %s WHERE item_id = $1"
)
