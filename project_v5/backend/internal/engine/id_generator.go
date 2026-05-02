package engine

import "math/rand"

const idChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const idLength = 5

// GenerateID returns a random 5-character ID drawn from [A-Za-z0-9]. Like
// v9's generateId, this carries no uniqueness guarantee — callers handle
// collisions if it matters.
func GenerateID() string {
	b := make([]byte, idLength)
	for i := range b {
		b[i] = idChars[rand.Intn(len(idChars))]
	}
	return string(b)
}
