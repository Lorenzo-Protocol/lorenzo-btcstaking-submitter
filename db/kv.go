package db

import (
	"errors"
	"gorm.io/gorm"
	"strconv"
)

// Set sets the value of a key in the database
func Set(db *gorm.DB, key string, value string) error {
	var cfg Config
	err := db.Model(&Config{}).Where("name = ?", key).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cfg.Name = key
			cfg.Value = value
			return db.Create(&cfg).Error
		}

		return err
	}

	return db.Model(&Config{}).Where("name = ?", key).Update("value", value).Error
}

// Get retrieves the value of a key from the database
func Get(db *gorm.DB, key string) (string, error) {
	var cfg Config
	err := db.Model(&Config{}).Where("name = ?", key).First(&cfg).Error
	if err != nil {
		return "", err
	}

	return cfg.Value, nil
}

// GetUint64 retrieves the value of a key from the database and converts it to an uint64
// If no found, return 0
func GetUint64(db *gorm.DB, key string) (uint64, error) {
	val, err := Get(db, key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}

		return 0, err
	}

	return strconv.ParseUint(val, 10, 64)
}

// SetUint64 sets uint64 value of a key in the database
func SetUint64(db *gorm.DB, key string, value uint64) error {
	return Set(db, key, strconv.FormatUint(value, 10))
}
