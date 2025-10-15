
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
go build -o /tmp/kms_oci_crypt .
```

- Test binary provider

Edit `example/ocicrypt.json` and enter the full path to the binary:

```json
{
    "key-providers": {
      "kmscrypt": {
        "cmd": {
          "path": "/tmp/kms_oci_crypt",
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


#### Encrypt

In a new shell, specify the path to the config file

```bash
cd example/
export OCICRYPT_KEYPROVIDER_CONFIG=`pwd`/ocicrypt.json
export SSL_CERT_FILE=`pwd`/certs/tls-ca-chain.pem

export PROJECT_ID=`gcloud config get-value core/project`

skopeo copy --encrypt-layer=-1 \
  --encryption-key=provider:kmscrypt:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/ocikeyring/cryptoKeys/key1 \
   docker://docker.io/salrashid123/app docker://localhost:5000/app:encrypted
```

The last layer on the image shjould be encrypted 

```bash
skopeo inspect docker://localhost:5000/app:encrypted
```

```json
{
    "Name": "localhost:5000/app",
    "Digest": "sha256:a1ae699cb116637ad5e4b18019622130a6b145e9b9ba2ecf258ca916705a79de",
    "RepoTags": [
        "encrypted"
    ],
    "Created": "2025-10-14T02:53:18.980326736-04:00",
    "DockerVersion": "",
    "Labels": null,
    "Architecture": "amd64",
    "Os": "linux",
    "Layers": [
        "sha256:dd5ad9c9c29f04b41a0155c720cf5ccab28ef6d353f1fe17a06c579c70054f0a",
        "sha256:960043b8858c3c30f1d79dcc49adb2804fd35c2510729e67685b298b2ca746b7",
        "sha256:b4ca4c215f483111b64ec6919f1659ff475d7080a649d6acd78a6ade562a4a63",
        "sha256:eebb06941f3e57b2e40a0e9cbd798dacef9b04d89ebaa8896be5f17c976f8666",
        "sha256:02cd68c0cbf64abe9738767877756b33f50fff5d88583fdc74b66beffa77694b",
        "sha256:d3c894b5b2b0fa857549aeb6cbc38b038b5b2828736be37b6d9fff0b886f12fd",
        "sha256:b40161cd83fc5d470d6abe50e87aa288481b6b89137012881d74187cfbf9f502",
        "sha256:46ba3f23f1d3fb1440deeb279716e4377e79e61736ec2227270349b9618a0fdd",
        "sha256:4fa131a1b726b2d6468d461e7d8867a2157d5671f712461d8abd126155fdf9ce",
        "sha256:01f38fc88b34d9f2e43240819dd06c8b126eae8a90621c1f2bc5042fed2b010a",
        "sha256:50891eb6c2e685b267299b99d8254e5b0f30bb7756ee2813f187a29a0a377247",
        "sha256:c4cd914051cf67617ae54951117708987cc63ce15f1139dee59abf80c198b74e",
        "sha256:ddc9bffd9ec055efa68358f5e2138ed78a2226694db2b8e1fb46c4ae84bae5ad",
        "sha256:7039b57e7e0592f8d78bee3eaac566ddb6ab100c04807301cc78a83ecde3ef62"
    ],
    "LayersData": [
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:dd5ad9c9c29f04b41a0155c720cf5ccab28ef6d353f1fe17a06c579c70054f0a",
            "Size": 83932,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:960043b8858c3c30f1d79dcc49adb2804fd35c2510729e67685b298b2ca746b7",
            "Size": 20322,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:b4ca4c215f483111b64ec6919f1659ff475d7080a649d6acd78a6ade562a4a63",
            "Size": 599551,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:eebb06941f3e57b2e40a0e9cbd798dacef9b04d89ebaa8896be5f17c976f8666",
            "Size": 284,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:02cd68c0cbf64abe9738767877756b33f50fff5d88583fdc74b66beffa77694b",
            "Size": 188,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:d3c894b5b2b0fa857549aeb6cbc38b038b5b2828736be37b6d9fff0b886f12fd",
            "Size": 112,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:b40161cd83fc5d470d6abe50e87aa288481b6b89137012881d74187cfbf9f502",
            "Size": 382,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:46ba3f23f1d3fb1440deeb279716e4377e79e61736ec2227270349b9618a0fdd",
            "Size": 345,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:4fa131a1b726b2d6468d461e7d8867a2157d5671f712461d8abd126155fdf9ce",
            "Size": 122108,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:01f38fc88b34d9f2e43240819dd06c8b126eae8a90621c1f2bc5042fed2b010a",
            "Size": 5209711,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:50891eb6c2e685b267299b99d8254e5b0f30bb7756ee2813f187a29a0a377247",
            "Size": 1889065,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:c4cd914051cf67617ae54951117708987cc63ce15f1139dee59abf80c198b74e",
            "Size": 921781,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:ddc9bffd9ec055efa68358f5e2138ed78a2226694db2b8e1fb46c4ae84bae5ad",
            "Size": 4156535,
            "Annotations": null
        },
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip+encrypted",
            "Digest": "sha256:7039b57e7e0592f8d78bee3eaac566ddb6ab100c04807301cc78a83ecde3ef62",
            "Size": 135,
            "Annotations": {
                "org.opencontainers.image.enc.keys.provider.kmscrypt": "eyJrZXlfdXJsIjoiZ2Nwa21zOi8vcHJvamVjdHMvY29yZS1lc28vbG9jYXRpb25zL2dsb2JhbC9rZXlSaW5ncy9vY2lrZXlyaW5nL2NyeXB0b0tleXMva2V5MSIsIndyYXBwZWRfa2V5IjoiQ2lRQW5mbU15UWFqRldPNHlPWjZhaC9iaUU5N0dncVdLaEsvVFZlVWxqcjkzVCtoTE1ZUzZ3RUFUbk1adEdhSVBQT25FS3I1VDdmUk9YcnRWNEJ1QkhRWkVoa1JRdjFRNGxabm9lYS9aZ1lteTRjRXpRN2Nzc2NybmhoOVVvejlLdFlZendrVUREenY2QUdpWXpxbjZJbnJCckFhNzByUXZCZDZSTzNLL1NFdXBxNHdjZlY5dlpjWkQ2RmY1cHZRWEppQlplaWYzdDhkblE1M0NDem15eE9mSW1VTnI4VGYyQ0pac1lEbjR5MXlEdEF6bFlHTE4rbGs2OTBJVUkzaXV3WDcxY2dZWHB3OUloZDdPanBWdHExSkhRaTd0d3M2SUNWbmcwbDNmL2xRN1BXdDFKL0lXWHNKT2NOUHlxOFNMM2pqVDVyQnEvRE5LeEFmUktlNmxkUUxKZXF4Rk9FK2d1LzVrQnRNZmxXa1ZkWU4iLCJ3cmFwX3R5cGUiOiJBRVMifQ==",
                "org.opencontainers.image.enc.pubopts": "eyJjaXBoZXIiOiJBRVNfMjU2X0NUUl9ITUFDX1NIQTI1NiIsImhtYWMiOiJNZ0JUQnB0cERHTG9jZ3dtNENMVTRRam1WRGQzU29MQVAzQzMxM3lpenhZPSIsImNpcGhlcm9wdGlvbnMiOnt9fQ=="
            }
        }
    ],
    "Env": [
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        "SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt"
    ]
}
```

Note the last layer is encrypted

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
   docker://docker.io/salrashid123/app docker://localhost:5000/app:encrypted

skopeo copy --dest-tls-verify=false \
  --decryption-key=provider:grpc-keyprovider:gcpkms://projects/$PROJECT_ID/locations/global/keyRings/ocikeyring/cryptoKeys/key1 \
    docker://localhost:5000/app:encrypted docker://localhost:5000/app:decrypted
```

