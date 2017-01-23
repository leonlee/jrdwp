package common

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"time"
)

//GenerateSecret generate secret calculated by unixtime without seconds
func GenerateSecret() []byte {
	secret := fmt.Sprintf("%d", time.Now().Unix())

	return []byte(secret)
}

//ParseSecret parse secret to unixtime
func ParseSecret(secret []byte) int64 {
	str := string(secret)
	unixtime, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		log.Println("bad unixtime", str, err.Error())
		return 0
	}
	return unixtime
}

//GenerateKey generate encrypt key
func GenerateKey(key *rsa.PublicKey) string {
	secret := GenerateSecret()
	log.Println("generated secret", string(secret))

	token := hex.EncodeToString(EncryptSecret(key, secret))
	log.Println("generated token", token)

	return token
}

//EncryptSecret encrypt secret with private key
func EncryptSecret(key *rsa.PublicKey, secret []byte) []byte {
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, key, secret, []byte{})
	if err != nil {
		log.Fatalln("can't encrypt secret", err.Error())
	}
	return ciphertext
}

//ParsePublicKey parse rsa public key
func ParsePublicKey(bytes []byte) *rsa.PublicKey {
	block, _ := pem.Decode(bytes)
	if block == nil {
		log.Fatalln("can't decode pem", string(bytes))
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		log.Fatalln("can't parse public key", err.Error())
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		log.Fatalln("invalid public key", reflect.TypeOf(key))
	}

	return rsaKey
}

//PublicKeyToBytes convert rsa.PublicKey to bytes
func PublicKeyToBytes(publicKey *rsa.PublicKey) ([]byte, error) {
	publicKeyDer, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		log.Fatalln("can't marshal public key", err.Error())
		return nil, err
	}

	publicKeyBlock := pem.Block{
		Type:    "RSA PUBLIC KEY",
		Headers: nil,
		Bytes:   publicKeyDer,
	}

	return pem.EncodeToMemory(&publicKeyBlock), nil
}

//VerifyToken verify the token
func VerifyToken(key *rsa.PrivateKey, token string) bool {
	if key == nil {
		log.Println("bad private key")
		return false
	}

	if token == "" {
		log.Println("empty token")
		return false
	}

	tokenBytes, err := hex.DecodeString(string(token))
	if err != nil {
		log.Println("can't decode token:", string(token), err.Error())
		return false
	}

	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, tokenBytes, []byte{})
	if err != nil {
		log.Println("can't decrypt token", err.Error())
		return false
	}

	timestamp := ParseSecret(plaintext)
	unixtime := time.Now().Unix()

	return timestamp >= unixtime-60
}
