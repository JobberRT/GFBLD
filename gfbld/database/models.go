package database

type LiveRecord struct {
	Id         int    `gorm:"primaryKey;autoIncrement"`
	LiveId     string `gorm:"live_id"`
	CreateTime string `gorm:"create_time"`
	UploadTime string `gorm:"upload_time"`
	Name       string `gorm:"name"`
	Combined   bool   `gorm:"combined"`
}

type Video struct {
	Id         int    `gorm:"primaryKey;autoIncrement"`
	BelongsTo  string `gorm:"belongs_to"`
	CreateTime string `gorm:"create_time"`
	Name       string `gorm:"name"`
	Url        string `gorm:"url"`
	Downloaded bool   `gorm:"downloaded"`
	FileName   string `gorm:"filename"`
}

type Audio struct {
	Id         int    `gorm:"primaryKey;autoIncrement"`
	BelongsTo  string `gorm:"belongs_to"`
	CreateTime string `gorm:"create_time"`
	Name       string `gorm:"name"`
	Url        string `gorm:"url"`
	Downloaded bool   `gorm:"downloaded"`
	FileName   string `gorm:"filename"`
}
