package db

import (
	"errors"
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"time"
)

type ConfigTable struct {
	Id    int
	Name  string
	Value string

	UpdatedTime time.Time `gorm:"autoUpdateTime"`
	CreatedTime time.Time `gorm:"autoCreateTime"`
}

func (ConfigTable) TableName() string {
	return "config"
}

type MysqlDB struct {
	db *gorm.DB
}

func NewMysqlDB(host string, port int, user string, password string, dbname string) (*MysqlDB, error) {
	dns := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, host, port, dbname)
	db, err := gorm.Open(mysql.Open(dns), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	mysqlDb := &MysqlDB{
		db: db,
	}

	return mysqlDb, nil
}

func (db *MysqlDB) Put(key []byte, value []byte) error {
	var cfg ConfigTable
	cfg.Name = string(key)
	cfg.Value = string(value)

	results := db.db.Model(&ConfigTable{}).Where("name = ?", string(key)).First(&cfg)
	if results.Error != nil {
		if errors.Is(results.Error, gorm.ErrRecordNotFound) {
			return db.db.Create(&cfg).Error
		}
	}

	return db.db.Model(&ConfigTable{}).Where("name = ?", cfg.Name).Updates(cfg).Error
}

func (db *MysqlDB) Delete(key []byte) error {
	return db.db.Model(&ConfigTable{}).Where("name = ?", string(key)).Delete(&ConfigTable{}).Error
}

func (db *MysqlDB) Has(key []byte) (bool, error) {
	err := db.db.Model(&ConfigTable{}).Where("name = ?", string(key)).First(&ConfigTable{}).Error
	return err == nil, err
}

func (db *MysqlDB) Get(key []byte) ([]byte, error) {
	var cfg ConfigTable
	err := db.db.Model(&ConfigTable{}).Where("name = ?", string(key)).First(&cfg).Error
	if err != nil {
		return nil, err
	}

	return []byte(cfg.Value), nil
}
