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
)

const (
	kmsCryptName = "kmscrypt"
)

var (
	kmsClient *kms.KeyManagementClient
	adc       = flag.String("adc", "", "Path to ADC file")
	kmsURI    = flag.String("kmsURI", "", "Path to kms URI")
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

	if *adc != "" {
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", *adc)
		//kmsClient, err = kms.NewKeyManagementClient(ctx, option.WithCredentialsFile(*adc))
	}
	kmsClient, err = kms.NewKeyManagementClient(ctx)
	if err != nil {
		log.Fatal("Error initalizing KMS client", err)
	}
	defer kmsClient.Close()

	var input keyprovider.KeyProviderKeyWrapProtocolInput
	err = json.NewDecoder(os.Stdin).Decode(&input)
	if err != nil {
		log.Fatal("decoding input", err)
	}

	switch input.Operation {
	case keyprovider.OpKeyWrap:
		if *kmsURI != "" {
			myMap := make(map[string][][]byte)
			myMap["kmscrypt"] = [][]byte{[]byte(*kmsURI)}
			input.KeyWrapParams.Ec.Parameters = myMap
		}

		b, err := WrapKey(input)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s", b)
	case keyprovider.OpKeyUnwrap:
		if *kmsURI != "" {
			myMap := make(map[string][][]byte)
			myMap["kmscrypt"] = [][]byte{[]byte(*kmsURI)}
			input.KeyUnwrapParams.Dc.Parameters = myMap
		}

		b, err := UnwrapKey(input)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s", b)
	default:
		log.Fatalf("Operation %v not recognized", input.Operation)
	}
}

func WrapKey(keyP keyprovider.KeyProviderKeyWrapProtocolInput) ([]byte, error) {

	_, ok := keyP.KeyWrapParams.Ec.Parameters[kmsCryptName]
	if !ok {
		return nil, errors.New("provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	if len(keyP.KeyWrapParams.Ec.Parameters[kmsCryptName]) == 0 {
		return nil, errors.New("provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	kmsURI := string(keyP.KeyWrapParams.Ec.Parameters[kmsCryptName][0])
	kmsName := ""
	if strings.HasPrefix(kmsURI, "gcpkms://") {
		kmsName = strings.TrimPrefix(kmsURI, "gcpkms://")
	} else {
		return nil, fmt.Errorf("unsupported kms prefix %s", kmsURI)
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
		return nil, errors.New("decrypt Provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	if len(keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName]) == 0 {
		return nil, errors.New("decrypt Provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
	}

	kmsURI := string(keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName][0])
	kmsName := ""
	if strings.HasPrefix(kmsURI, "gcpkms://") {
		kmsName = strings.TrimPrefix(kmsURI, "gcpkms://")
	} else {
		return nil, fmt.Errorf("unsupported kms prefix %s", kmsURI)
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
