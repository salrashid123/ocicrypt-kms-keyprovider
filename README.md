
## OCICrypt Container Image KMS Provider

Basic [OCICrypt KeyProvider](https://github.com/containers/ocicrypt/blob/main/docs/keyprovider.md) for KMS (GCP only for now, AWS, Azure would be pretty easy to add).

This repo includes a prebuilt and customizeable keyprovider which can be used to encrypt OCI Containers.

[OCICrypt](https://github.com/containers/ocicrypt) includes specifications to encrypt an OCI Container image and within that, the keyprovider protocol allows wrapping of the actual key used to encrypt the layer to an external binary.

The binary in this question accepts a keyprovider request and inturn wraps the layer symmetric encryption key using a hosted KMS key.

Basically, KMS wraps the symmetric key that is used to encrypt the layer itself.

This sample is based off of the [simple-oci-keyprovider](https://github.com/lumjjb/simple-ocicrypt-keyprovider.git) repo which demonstrates the protocol involved.

For more information, see 

- [Advancing container image security with encrypted container images](https://developer.ibm.com/articles/advancing-image-security-encrypted-container-images/)
- [Enabling advanced key usage and management in encrypted container images](https://developer.ibm.com/articles/enabling-advanced-key-usage-and-management-in-encrypted-container-images/)
- [Container Image Encryption & Decryption in the CoCo project](https://medium.com/kata-containers/confidential-containers-and-encrypted-container-images-fc4cdb332dec)

Note, for KMS and other systems, you can also use built in [PKCS11 support](https://github.com/containers/ocicrypt/blob/main/docs/pkcs11.md).  If you are interested in using a `Trusted Platform Module` as the root encryption source, see [Container Signing with Cosign and TPM PKCS-11](https://blog.salrashid.dev/articles/2022/cosign_tpm/) and [PKCS 11 Samples in Go using SoftHSM](https://github.com/salrashid123/go_pkcs11).


Anyway, this repo shows basic OCI container encryption and then an example with GCP KMS as the key wrapping provider.

* [Setup Baseline](#setup-baseline)
* [Setup Binary OCI KMS provider](#setup-binary-oci-kms-provider)
* [Setup gRPC OCI KMS provider](#setup-grpc-oci-kms-provider)

![images/encrypted.png](images/encrypted.png)

---

### Setup

Showing how this works involves a number of steps so its not that much of a quickstart but once its setup, you can skip to the "encrypt/decrypt" section below.

install

- [skopeo](https://github.com/containers/skopeo/blob/main/install.md)
- [crane](https://github.com/google/go-containerregistry/blob/main/cmd/crane/README.md)
- docker


#### Setup Binary OCI KMS provider

Create a KMS Key to use on GCP

```bash
export PROJECT_ID=`gcloud config get-value core/project`
export PROJECT_NUMBER=`gcloud projects describe $PROJECT_ID --format='value(projectNumber)'`
export GCLOUD_USER=`gcloud config get-value core/account`

gcloud auth application-default login

gcloud kms keyrings create ocikeyring --location=global

gcloud kms keys create key1 --keyring=ocikeyring  --location=global --purpose=encryption

# this is unnecessary since you should already have permissions
gcloud kms keys add-iam-policy-binding key1    \
    --keyring=ocikeyring --location=global \
     --member="user:$GCLOUD_USER" --role=roles/cloudkms.cryptoKeyEncrypterDecrypter
```

#### Build plugin

(or download the binary from the "releases" page)

```bash
cd plugin
go build -o kms_oci_crypt .
```

- Test binary provider

Edit `example/ocicrypt.json` and enter the full path to the binary:

```json
{
    "key-providers": {
      "kmscrypt": {
        "cmd": {
          "path": "/full/path/to/kms_oci_crypt",
          "args": []
        }
      }
    }
}
```

#### Run local Registry

Run a local docker registry just to test (vs docker-daemon)

```bash
cd example

docker run  -p 5000:5000 -v `pwd`/certs:/certs \
  -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/localhost.crt \
  -e REGISTRY_HTTP_TLS_KEY=/certs/localhost.key  docker.io/registry:2
```

#### Build and push a test image

Build a small app and push to local reg

```bash
cd example

docker build -t app:server .

export SSL_CERT_FILE=certs/tls-ca-chain.pem
skopeo copy   docker-daemon:app:server  docker://localhost:5000/app:server
```


#### Encrypt

In a new shell, specify the path to the config file

```bash
export OCICRYPT_KEYPROVIDER_CONFIG=/full/path/to/ocicrypt.json
```

Then encrypt the last layer

```bash
export PROJECT_ID=`gcloud config get-value core/project`

skopeo copy --encrypt-layer=-1 \
  --encryption-key=provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/ocikeyring/cryptoKeys/key1 \
   docker://localhost:5000/app:server docker://localhost:5000/app:encrypted
```

The last layer on the image shjould be encrypted 

```bash
skopeo inspect docker://localhost:5000/app:encrypted
```

#### Decrypt

```bash
export PROJECT_ID=`gcloud config get-value core/project`

skopeo copy \
  --decryption-key=provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/ocikeyring/cryptoKeys/key1 \
   docker://localhost:5000/app:encrypted docker://localhost:5000/app:decrypted
```

Inspect the decrypted image

```bash
skopeo inspect docker://localhost:5000/app:decrypted
```

#### Configuring ADC

Finally, you can specify the path GCP `Application Default Credentials` file by setting the startup argument `--adc`.  You can use this setting to direct the GCP encryption to use [Workload Identity Federation](https://cloud.google.com/iam/docs/workload-identity-federation)

```json
{
  "key-providers": {
    "kmscrypt": {
      "cmd": {
        "path": "/path/to/kms_oci_crypt",
        "args": [
          "--adc=/path/to/application_default_credentials.json"
        ]
      }
    }
  }
}
```

---

### Setup gRPC OCI KMS provider

Included in this repo is a grpc service which you can use as the key provider.

Basically, its the same as calling the binary except that it calls a gRPC server you run separately.

Note, the existing implementation _does not use TLS_!.  You would definitely want to secure access to this service.

To use, start the server

```bash
cd grpc

go run server.go
```

set the `OCICRYPT_KEYPROVIDER_CONFIG` file to use

```json
{
  "key-providers": {
    "kmscrypt": {
      "cmd": {
        "path": "/path/to/kms_oci_crypt",
        "args": []
      }
    },
    "grpc-keyprovider": {
      "grpc": "localhost:50051"
    }
  }
}
```

Finally invoke the endpoints (note `provider:grpc-keyprovider` is used below)

```bash
cd example/
export SSL_CERT_FILE=certs/tls-ca-chain.pem

skopeo copy --encrypt-layer -1 \
  --encryption-key=provider:grpc-keyprovider:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/ocikeyring/cryptoKeys/key1 \
   docker://localhost:5000/app:server docker://localhost:5000/app:encrypted

skopeo copy --dest-tls-verify=false \
  --decryption-key=provider:grpc-keyprovider:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/ocikeyring/cryptoKeys/key1 \
    docker://localhost:5000/app:encrypted docker://localhost:5000/app:decrypted
```

