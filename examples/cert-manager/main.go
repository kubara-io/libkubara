package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/kubara-io/libkubara/manifest"
)

//go:embed issuer-crd.yaml
var issuerCRDYAML []byte

//go:embed issuer-valid.yaml
var issuerValidYAML []byte

//go:embed issuer-invalid.yaml
var issuerInvalidYAML []byte

func main() {
	ctx := context.Background()

	validator, err := loadValidator(issuerCRDYAML)
	check(err)

	fmt.Println("=== Scenario 1: Prevalidate valid Issuer & parse into certmanagerv1.Issuer ===")
	issuer, err := validateAndParseIssuer(ctx, validator, issuerValidYAML)
	check(err)

	fmt.Printf("Validated CR: %s/%s\n", issuer.Namespace, issuer.Name)
	fmt.Printf("ACME Server: %s\n", issuer.Spec.ACME.Server)
	fmt.Printf("ACME Email: %s\n", issuer.Spec.ACME.Email)
	fmt.Printf("ACME Secret: %s (key: %s)\n", issuer.Spec.ACME.PrivateKey.Name, issuer.Spec.ACME.PrivateKey.Key)
	fmt.Printf("Solvers count: %d\n\n", len(issuer.Spec.ACME.Solvers))

	fmt.Println("=== Scenario 2: Prevalidate invalid Issuer (missing required fields) ===")
	_, err = validateAndParseIssuer(ctx, validator, issuerInvalidYAML)
	if err == nil {
		panic("expected invalid issuer to fail validation")
	}
	fmt.Printf("Local validation error caught:\n%v\n", err)
}

func loadValidator(crdBytes []byte) (*crdvalidate.Validator, error) {
	return crdvalidate.Compile(bytes.NewReader(crdBytes))
}

func validateAndParseIssuer(ctx context.Context, validator *crdvalidate.Validator, crYAML []byte) (*certmanagerv1.Issuer, error) {
	rawObj, err := manifest.DecodeOneBytes(crYAML)
	if err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	result := validator.ValidateCreate(ctx, rawObj, crdvalidate.RejectUnknown)

	var issuer certmanagerv1.Issuer
	if err := result.Into(&issuer); err != nil {
		return nil, fmt.Errorf("validate CR: %w", err)
	}

	return &issuer, nil
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
