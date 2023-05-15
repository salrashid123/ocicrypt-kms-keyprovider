package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"flag"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/containers/ocicrypt/keywrap/keyprovider"
	"google.golang.org/api/option"
)

const (
	kmsCryptName = "kmscrypt"
)

var (
	kmsClient *kms.KeyManagementClient
	adc       = flag.String("adc", "", "Path to ADC file")
)

type annotationPacket struct {
	KeyUrl     string `json:"key_url"`
	WrappedKey []byte `json:"wrapped_key"`
	WrapType   string `json:"wrap_type"`
}

func main() {

	flag.Parse()
	ctx := context.Background()
	var err error
	if *adc == "" {
		kmsClient, err = kms.NewKeyManagementClient(ctx)
	} else {
		dat, err := os.ReadFile(*adc)
		if err != nil {
			log.Fatal("decoding input", err)
		}
		kmsClient, err = kms.NewKeyManagementClient(ctx, option.WithCredentialsJSON(dat))
	}
	if err != nil {
		log.Fatal("Error initalizing KMS client", err)
	}

	var input keyprovider.KeyProviderKeyWrapProtocolInput
	err = json.NewDecoder(os.Stdin).Decode(&input)
	if err != nil {
		log.Fatal("decoding input", err)
	}

	if input.Operation == keyprovider.OpKeyWrap {
		b, err := WrapKey(input)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s", b)
	} else if input.Operation == keyprovider.OpKeyUnwrap {
		b, err := UnwrapKey(input)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s", b)
	} else {
		log.Fatalf("Operation %v not recognized", input.Operation)
	}
	return
}

func WrapKey(keyP keyprovider.KeyProviderKeyWrapProtocolInput) ([]byte, error) {

	_, ok := keyP.KeyWrapParams.Ec.Parameters[kmsCryptName]
	if !ok {
		return nil, errors.New("Provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	if len(keyP.KeyWrapParams.Ec.Parameters[kmsCryptName]) == 0 {
		return nil, errors.New("Provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	kmsURI := string(keyP.KeyWrapParams.Ec.Parameters[kmsCryptName][0])
	kmsName := ""
	if strings.HasPrefix(kmsURI, "gcpkms://") {
		kmsName = strings.TrimPrefix(kmsURI, "gcpkms://")
	} else {
		return nil, fmt.Errorf("Unsupported kms prefix %s", kmsURI)
	}
	ctx := context.Background()

	req := &kmspb.EncryptRequest{
		Name:                        kmsName,
		Plaintext:                   keyP.KeyWrapParams.OptsData,
		AdditionalAuthenticatedData: nil,
	}

	result, err := kmsClient.Encrypt(ctx, req)
	if err != nil {
		return nil, err
	}

	// *****************************************

	jsonString, err := json.Marshal(annotationPacket{
		KeyUrl:     kmsURI,
		WrappedKey: result.Ciphertext,
		WrapType:   "AES",
	})
	if err != nil {
		return nil, err
	}

	return json.Marshal(keyprovider.KeyProviderKeyWrapProtocolOutput{
		KeyWrapResults: keyprovider.KeyWrapResults{
			Annotation: jsonString,
		},
	})

}

func UnwrapKey(keyP keyprovider.KeyProviderKeyWrapProtocolInput) ([]byte, error) {
	apkt := annotationPacket{}
	err := json.Unmarshal(keyP.KeyUnwrapParams.Annotation, &apkt)
	if err != nil {
		return nil, err
	}
	//kmsURI := apkt.KeyUrl
	ciphertext := apkt.WrappedKey

	_, ok := keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName]
	if !ok {
		return nil, errors.New("Provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	if len(keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName]) == 0 {
		return nil, errors.New("Provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	kmsURI := string(keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName][0])
	kmsName := ""
	if strings.HasPrefix(kmsURI, "gcpkms://") {
		kmsName = strings.TrimPrefix(kmsURI, "gcpkms://")
	} else {
		return nil, fmt.Errorf("Unsupported kms prefix %s", kmsURI)
	}
	ctx := context.Background()
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	req := &kmspb.DecryptRequest{
		Name:                        kmsName,
		Ciphertext:                  ciphertext,
		AdditionalAuthenticatedData: nil,
	}

	result, err := kmsClient.Decrypt(ctx, req)
	if err != nil {
		return nil, err
	}

	return json.Marshal(keyprovider.KeyProviderKeyWrapProtocolOutput{
		KeyUnwrapResults: keyprovider.KeyUnwrapResults{OptsData: result.Plaintext},
	})
}
