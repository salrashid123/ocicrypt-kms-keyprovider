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
	debugLog  = flag.String("debugLog", "", "Path to debuglog")
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

	if *debugLog != "" {
		file, err := os.OpenFile(*debugLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Printf("error opening log file: %v", err)
		}
		defer file.Close()
		log.SetOutput(file)
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	}

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

		// if the user specified it in command line, set that as the parameter value
		if *kmsURI != "" {
			if len(input.KeyWrapParams.Ec.Parameters) == 0 {
				input.KeyWrapParams.Ec.Parameters = make(map[string][][]byte)
			}
			input.KeyWrapParams.Ec.Parameters[kmsCryptName] = [][]byte{[]byte(*kmsURI)}
		}

		b, err := WrapKey(input)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s", b)
	case keyprovider.OpKeyUnwrap:

		// if the user specified it in command line, set that as the parameter value
		if *kmsURI != "" {
			if len(input.KeyUnwrapParams.Dc.Parameters) == 0 {
				input.KeyUnwrapParams.Dc.Parameters = make(map[string][][]byte)
			}
			input.KeyUnwrapParams.Dc.Parameters[kmsCryptName] = [][]byte{[]byte(*kmsURI)}
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

	// load up the keyURL if its in the packet
	kmsURI := apkt.KeyUrl
	ciphertext := apkt.WrappedKey

	// now load it from the parameter; the paramater has the saved value the user specified in the commandline args
	//  the parameter value should take precedent over apkt.KeyUrl
	_, ok := keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName]
	if ok {
		if len(keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName]) == 0 && apkt.KeyUrl == "" {
			return nil, errors.New("decrypt Provider must be formatted as provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/[keyring]/cryptoKeys/[key]/cryptoKeyVersions/1")
		}
		kmsURI = string(keyP.KeyUnwrapParams.Dc.Parameters[kmsCryptName][0])
	}

	if kmsURI == "" {
		return nil, errors.New("kmsURI cannot be nil")
	}

	if kmsURI != apkt.KeyUrl {
		return nil, fmt.Errorf("kmsURI parameter and keyURL in structure are different parameter [%s], keyURL [%s]", kmsURI, apkt.KeyUrl)
	}

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
