package main

import (
	"time"
)

type itemID struct {
	ItemID string `db:"item_id" json:"item_id" uri:"item_id" binding:"required,alphanum,len=8"`
}

type itemData struct {
	ItemCount uint64  `db:"item_count" json:"item_count" binding:"required,numeric"`
	ItemPrice float64 `db:"item_price" json:"item_price" binding:"required,numeric"`
	ItemBrand string  `db:"item_brand" json:"item_brand" binding:"required,ascii`
	ItemName  string  `db:"item_name" json:"item_name" binding:"required,ascii"`
	ItemDesc  string  `db:"item_desc" json:"item_desc" binding:"required,ascii"`
}

type inventoryRow struct {
	itemID
	CreatedAt time.Time `db:"created_at" json:"created_at" binding:"required,datetime" time_format:"unix"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at" binding:"required,datetime" time_format:"unix"`
	itemData
}

type itemImage struct {
	ImgBase64 string `json:"image_base64" binding:"required,base64"`
}

type apiRequestListQuery struct {
	OrderBy string `form:"order_by,default=updated_at" binding:"oneof=created_at updated_at"`
	Order   string `form:"order,default=desc" binding:"oneof=asc desc"`
}

type imgRequestGetQuery struct {
	Height uint16 `form:"h,default=0" binding:"number,gte=0,lte=8192"`
	Width  uint16 `form:"w,default=0" binding:"number,gte=0,lte=8192"`
}

type apiRequestAddBody struct {
	itemData
	itemImage
}

type apiRequestUpdateQuery struct {
	UpdateField string `form:"update_field" binding:"required,oneof=item_count item_price item_brand item_name item_desc image_base64"`
}

type apiRequestUpdateBody struct {
	ItemCount   uint64  `db:"item_count,omitempty" json:"item_count" binding:"numeric"`
	ItemPrice   float64 `db:"item_price,omitempty" json:"item_price" binding:"numeric"`
	ItemBrand   string  `db:"item_brand,omitempty" json:"item_brand" binding:"ascii`
	ItemName    string  `db:"item_name,omitempty" json:"item_name" binding:"ascii"`
	ItemDesc    string  `db:"item_desc,omitempty" json:"item_desc" binding:"ascii"`
	ImageBase64 string  `json:"image_base64,omitempty" binding:"ascii"`
}

type apiResponse struct {
	Data  interface{} `json:"data"`
	Error interface{} `json:"error"`
}
