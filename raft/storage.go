package raft

import "sync"

type Storage interface { // Storage is the interface for the storage of Raft
	Set(key string, value []byte)  // Set stores the value of the given key
	Get(key string) ([]byte, bool) // Get returns the value of the given key and a boolean value indicating whether the key was found
	HasData() bool                 // HasData returns a boolean value indicating whether there is any data in the storage
}

// Simple implementation of the Storage interface in memory
type Database struct { // Database is a storage implementation that uses a map as the underlying storage
	mu sync.Mutex        // Mutex protects the map
	kv map[string][]byte // kv is the map of the storage
}

func NewDatabase() *Database {
	newKV := make(map[string][]byte) // newKV is the map of the storage
	return &Database{                // Return a new Database
		kv: newKV,
	}
}

func (db *Database) Get(key string) ([]byte, bool) {
	db.mu.Lock()               // Lock the database
	defer db.mu.Unlock()       // Unlock the database
	value, found := db.kv[key] // Get the value of the key
	return value, found        // Return the value and a boolean value indicating whether the key was found
}

func (db *Database) Set(key string, value []byte) {
	db.mu.Lock()         // Lock the database
	defer db.mu.Unlock() // Unlock the database
	db.kv[key] = value   // Set the value of the key in the database
}

func (db *Database) HasData() bool { // HasData returns a boolean value indicating whether there is any data in the storage
	db.mu.Lock()          // Lock the database
	defer db.mu.Unlock()  // Unlock the database
	return len(db.kv) > 0 // Return a boolean value indicating whether there is any data in the storage
}
