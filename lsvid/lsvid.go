package lsvid

// Package developed by SPIFFE Assertions and Tokens WG, to provide, extend and validate Lightweight SVID.
// Ref document:
// https://docs.google.com/document/d/15rfAkzNTQa1ycs-fn9hyIYV5HbznPBsxB-f0vxhNJ24/edit?usp=drive_link

import (

	"context"
	"fmt"
	"encoding/base64"
	"encoding/json"
	"crypto"
	"crypto/rand"
	hash256 "crypto/sha256"
	"crypto/x509"
	"crypto/ecdsa"
	"log"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"github.com/spiffe/go-spiffe/v2/svid/jwtsvid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"time"

)

type LSVID struct {
	Token		*Token		`json:"token"`		// The LSVID document
	Bundle		*Token		`json:"bundle"`		// The Trust bundle document
}

type Token struct {	
	Nested		*Token		`json:"nested,omitempty"`
	Payload		*Payload	`json:"payload"`
	Signature	[]byte		`json:"signature"`
}

type Payload struct {
	Ver 		int8		`json:"ver,omitempty"`
	Alg 		string		`json:"alg,omitempty"`
	Iat			int64		`json:"iat,omitempty"`
	Iss			*IDClaim	`json:"iss,omitempty"`
	Sub			*IDClaim	`json:"sub,omitempty"`
	Aud			*IDClaim	`json:"aud,omitempty"`
}

type IDClaim struct {
	CN			string		`json:"cn,omitempty"` // e.g.: spiffe://example.org/workload
	PK			[]byte		`json:"pk,omitempty"` // e.g.: VGhpcyBpcyBteSBQdWJsaWMgS2V5
	ID			*Token		`json:"id,omitempty"` // e.g.: a complete LSVID
}

// lsvid -> string
func Encode(lsvid *Token) (string, error) {
	// Marshal the LSVID struct into JSON
	lsvidJSON, err := json.Marshal(lsvid)
	if err != nil {
		return "", fmt.Errorf("error marshaling LSVID to JSON: %v", err)
	}

	// Encode the JSON byte slice to Base64.RawURLEncoded string
	encLSVID := base64.RawURLEncoding.EncodeToString(lsvidJSON)

	return encLSVID, nil
}

// string -> lsvid
func Decode(encLSVID string) (*Token, error) {

	// fmt.Printf("LSVID to be decoded: %s\n", encLSVID)
    // Decode the base64.RawURLEncoded LSVID
    decoded, err := base64.RawURLEncoding.DecodeString(encLSVID)
    if err != nil {
        return nil, fmt.Errorf("error decoding LSVID: %v\n", err)
    }
	// fmt.Printf("Decoded LSVID to be unmarshaled: %s\n", decoded)

    // Unmarshal the decoded byte slice into your struct
    var decLSVID LSVID
    err = json.Unmarshal(decoded, &decLSVID)
    if err != nil {
        return nil, fmt.Errorf("error unmarshalling LSVID: %v\n", err)
    }

    return &decLSVID, nil
}

// Add the new Token to extend an existing one, and sign using provided key
func Extend(lsvid *Token, newPayload *agentPayload, key crypto.Signer) (string, error) {
	// TODO: Modify the payload struct to support custom claims (maybe using map[string]{interface})
	// Create the extended LSVID structure
	extLSVID := &Token{
		Nested:		lsvid,
		Payload:	newPayload,
	}

	// Marshal to JSON
	tmpToSign, err := json.Marshal(extLSVID)
	if err != nil {
		return "", fmt.Errorf("Error generating json: %v\n", err)
	} 

	// Sign extlSVID
	hash 	:= hash256.Sum256(tmpToSign)
	s, err := key.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return "", fmt.Errorf("Error generating signed assertion: %v\n", err)
	} 

	// Set extLSVID signature
	extLSVID.Signature = s

	// Encode signed LSVID
	outLSVID, err := Encode(extLSVID)
	if err != nil {
		return "", fmt.Errorf("Error encoding LSVID: %v\n", err)
	} 

	return outLSVID, nil

}

// Validate the given LSVID. 
// TODO: include the root (trust bundle) LSVID as parameter, to validate the inner signature
func Validate(lsvid *Token) (bool, error) {

	for (lsvid.Nested != nil) {

		// Check Aud -> Iss link
		if lsvid.Payload.Iss.CN != lsvid.Nested.Payload.Aud.CN {
			return false, fmt.Errorf("Aud -> Iss link validation failed\n")
		}
		fmt.Printf("Aud -> Iss link validation successful!\n")

		// Marshal the LSVID struct into JSON
		tmpLSVID := &Token{
			Nested:		lsvid.Nested,
			Payload:	lsvid.Payload,
		}
		lsvidJSON, err := json.Marshal(tmpLSVID)
		if err != nil {
		return false, fmt.Errorf("error marshaling LSVID to JSON: %v\n", err)
		}
		hash 	:= hash256.Sum256(lsvidJSON)

		// Parse the public key
		// TODO: Currently the pk is extracted from the iss lsvid that MUST be present
		// and we are still not validating the trust bundle.
		var issLSVID *Token
		issLSVID = lsvid.Payload.Iss.LS
		for (issLSVID.Nested != nil) {
			// fmt.Printf("Issuer nested LSVID found! %v\n", issLSVID.Nested)
			issLSVID = issLSVID.Nested
		}
		issLSSubPk, err := x509.ParsePKIXPublicKey(issLSVID.Payload.Sub.PK)
		if err != nil {
			return false, fmt.Errorf("Failed to parse public key: %v\n", err)
		}

		// validate the signature
		log.Printf("Verifying signature created by %s\n", lsvid.Payload.Iss.CN)
		verify := ecdsa.VerifyASN1(issLSSubPk.(*ecdsa.PublicKey), hash[:], lsvid.Signature)
		if verify == false {
			fmt.Printf("\nSignature validation failed!\n\n")
			return false, nil
		}
		log.Printf("Signature validation successful!\n")

		// jump to nested token
		lsvid = lsvid.Nested
	}

	// reached the inner most LSVID. 
	// Marshal the LSVID struct into JSON
	lsvidJSON, err := json.Marshal(lsvid.Payload)
	if err != nil {
	return false, fmt.Errorf("error marshaling LSVID to JSON: %v\n", err)
	}
	// log.Printf("Payload to be hashed: %s", lsvidJSON)
	hash 	:= hash256.Sum256(lsvidJSON)

	// Parse the public key
	issPk, err := x509.ParsePKIXPublicKey(lsvid.Payload.Iss.PK)
	if err != nil {
		return false, fmt.Errorf("Failed to parse public key: %v\n", err)
	}
	log.Printf("Public key to be used: %s", issPk)

	log.Printf("Verifying signature created by %s", lsvid.Payload.Iss.CN)
	verify := ecdsa.VerifyASN1(issPk.(*ecdsa.PublicKey), hash[:], lsvid.Signature)
	if verify == false {
		fmt.Printf("\nSignature validation failed!\n\n")
		return false, nil
	}

	return true, nil
}

// Fetch workload LSVID using modified FetchJWTSVID endpoint
func FetchLSVID(ctx context.Context, socketPath string) (string, error) {
	
	// Fetch claims data
	clientSVID, err := fetchSVID(ctx, socketPath)
	if err != nil {
		return "", fmt.Errorf("Unable to fetch X509 SVID: %v\n", err)
	}
	clientID 		:= clientSVID.ID.String()

	source, err := workloadapi.NewJWTSource(ctx, workloadapi.WithClientOptions(workloadapi.WithAddr(socketPath)))
	if err != nil {
		return "", fmt.Errorf("Unable to create JWTSource %v\n", err)
	}
	defer source.Close()
	
	fetchLSVID, err := source.FetchJWTSVID(ctx, jwtsvid.Params{
		Audience: clientID,
	})
	if err != nil {
		return "", fmt.Errorf("Unable to Fetch LSVID %v\n", err)
	}

	return fmt.Sprintf("%s", fetchLSVID.LSVID.Svid), nil
}

// Create an LSVID payload given a x509 certificate.
// PS: Payload claims are based in LSVID spec doc
func Cert2LSR(ctx context.Context, socketPath string, cert *x509.Certificate, audience string) (*Payload, error) {

	clientSVID, err := fetchSVID(ctx, socketPath)
	if err != nil {
		return &Payload{}, fmt.Errorf("Unable to fetch X509 SVID: %v\n", err)
	}
	clientID 		:= clientSVID.ID.String()

	// generate encoded public key
	tmppk, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return &Payload{}, err
	}
	// pubkey :=  base64.RawURLEncoding.EncodeToString(tmppk)

	// Versioning needs TBD. For poc, considering vr = 1
	if cert.URIs[0] == nil {
		return &Payload{}, fmt.Errorf("No certificate URI: %v", err)
	}
	sub := cert.URIs[0].String()
	// Create LSVID payload
	lsvidPayload := &Payload{
		Ver:	1,
		Alg:	"ES256",
		Iat:	time.Now().Round(0).Unix(),
		Iss:	&IDClaim{
			CN:	clientID,
		},
		Sub:	&IDClaim{
			CN:	sub,
			PK:	tmppk,
		},
		Aud:	&IDClaim{
			CN:	audience,
		},
	}

	return lsvidPayload, nil
}

// Fetch workload X509 SVID
func fetchSVID(ctx context.Context, socketPath string) (*x509svid.SVID, error) {

	// Create a `workloadapi.X509Source`, it will connect to Workload API using provided socket.
	source, err := workloadapi.NewX509Source(ctx, workloadapi.WithClientOptions(workloadapi.WithAddr(socketPath)))
	if err != nil {
		return nil, fmt.Errorf("Unable to create X509Source: %v", err)
	}
	defer source.Close()

	svid, err := source.GetX509SVID()
	if err != nil {
		return nil, fmt.Errorf("Unable to fetch SVID: %v", err)
	}

	return svid, nil
}