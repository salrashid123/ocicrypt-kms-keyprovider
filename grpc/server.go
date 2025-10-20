package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/containers/ocicrypt/keywrap/keyprovider"
	keyproviderpb "github.com/containers/ocicrypt/utils/keyprovider"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
)

var (
	grpcport  = flag.String("grpcport", ":50051", "grpcport")
	kmsClient *kms.KeyManagementClient
	adc       = flag.String("adc", "", "Path to ADC file")
	kmsURI    = flag.String("kmsURI", "", "Path to kms URI")
)

const (
	kmsCryptName = "grpc-keyprovider"
)

type server struct {
	keyproviderpb.UnimplementedKeyProviderServiceServer
}

type annotationPacket struct {
	KeyUrl     string `json:"key_url"`
	WrappedKey []byte `json:"wrapped_key"`
	WrapType   string `json:"wrap_type"`
}

func (*server) WrapKey(ctx context.Context, request *keyproviderpb.KeyProviderKeyWrapProtocolInput) (*keyproviderpb.KeyProviderKeyWrapProtocolOutput, error) {
	log.Println("got WrapKey")
	var keyP keyprovider.KeyProviderKeyWrapProtocolInput
	err := json.Unmarshal(request.KeyProviderKeyWrapProtocolInput, &keyP)
	if err != nil {
		return nil, err
	}

	if *kmsURI != "" {
		myMap := make(map[string][][]byte)
		myMap["kmscrypt"] = [][]byte{[]byte(*kmsURI)}
		keyP.KeyWrapParams.Ec.Parameters = myMap
	}
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

	req := &kmspb.EncryptRequest{
		Name:                        kmsName,
		Plaintext:                   keyP.KeyWrapParams.OptsData,
		AdditionalAuthenticatedData: nil,
	}

	result, err := kmsClient.Encrypt(ctx, req)
	if err != nil {
		return nil, err
	}

	jsonString, _ := json.Marshal(annotationPacket{
		KeyUrl:     kmsURI,
		WrappedKey: result.Ciphertext,
		WrapType:   "AES",
	})

	protocolOuputSerialized, _ := json.Marshal(keyprovider.KeyProviderKeyWrapProtocolOutput{
		KeyWrapResults: keyprovider.KeyWrapResults{Annotation: jsonString},
	})

	return &keyproviderpb.KeyProviderKeyWrapProtocolOutput{
		KeyProviderKeyWrapProtocolOutput: protocolOuputSerialized,
	}, nil
}

func (*server) UnWrapKey(ctx context.Context, request *keyproviderpb.KeyProviderKeyWrapProtocolInput) (*keyproviderpb.KeyProviderKeyWrapProtocolOutput, error) {
	log.Println("got UnWrapKey")
	var keyP keyprovider.KeyProviderKeyWrapProtocolInput
	err := json.Unmarshal(request.KeyProviderKeyWrapProtocolInput, &keyP)
	if err != nil {
		return nil, err
	}
	apkt := annotationPacket{}
	err = json.Unmarshal(keyP.KeyUnwrapParams.Annotation, &apkt)
	if err != nil {
		return nil, err
	}
	//kmsURI := apkt.KeyUrl
	ciphertext := apkt.WrappedKey

	if *kmsURI != "" {
		myMap := make(map[string][][]byte)
		myMap["kmscrypt"] = [][]byte{[]byte(*kmsURI)}
		keyP.KeyUnwrapParams.Dc.Parameters = myMap
	}

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
	protocolOuputSerialized, _ := json.Marshal(keyprovider.KeyProviderKeyWrapProtocolOutput{
		KeyUnwrapResults: keyprovider.KeyUnwrapResults{OptsData: result.Plaintext},
	})
	return &keyproviderpb.KeyProviderKeyWrapProtocolOutput{
		KeyProviderKeyWrapProtocolOutput: protocolOuputSerialized,
	}, nil
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
		log.Fatal("Error initializing KMS client", err)
	}

	lis, err := net.Listen("tcp", *grpcport)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	if *kmsURI == "" {
		log.Fatal("--kmsURI must be set")
	}

	myMap := make(map[string][][]byte)
	myMap["kmscrypt"] = [][]byte{[]byte(*kmsURI)}

	sopts := []grpc.ServerOption{grpc.MaxConcurrentStreams(10)}
	sopts = append(sopts)

	s := grpc.NewServer(sopts...)
	keyproviderpb.RegisterKeyProviderServiceServer(s, &server{})

	log.Printf("Starting gRPC Server at %s", *grpcport)
	s.Serve(lis)

}
