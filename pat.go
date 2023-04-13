// Copyright 2022 Contributors to the Veraison project.
// SPDX-License-Identifier: Apache-2.0

package parsectpm

import (
	"bytes"
	"crypto"
	"fmt"

	tpm2 "github.com/google/go-tpm/tpm2"
)

// PAT is a structure to wrap Platform Attestation Token
type PAT struct {
	TpmVer     *string `cbor:"tpmVer" json:"tpmVer"`
	KID        *[]byte `cbor:"kid" json:"kid"`
	Sig        *[]byte `cbor:"sig" json:"sig"` // This is TPMT_SIGNATURE
	AttestInfo *[]byte `cbor:"certInfo" json:"attestInfo"`
}

func NewPAT() *PAT {
	return &PAT{}
}

func (p *PAT) SetTpmVer(v string) error {
	if v == "" {
		return fmt.Errorf("empty string specified")
	}
	p.TpmVer = &v
	return nil
}

func (p *PAT) SetSig(s []byte) error {
	p.Sig = &s
	return nil
}

func (p *PAT) SetKeyID(v []byte) error {

	if err := validateKID(v); err != nil {
		return fmt.Errorf("invalid KID : %w", err)
	}
	p.KID = &v
	return nil
}

func (p PAT) Validate() error {
	if p.TpmVer == nil {
		return fmt.Errorf("TPM Version not set")
	} else if *p.TpmVer == "" {
		return fmt.Errorf("Empty TPM Version")
	}

	if p.KID == nil {
		return fmt.Errorf("missing key identifier")
	}

	if err := validateKID(*p.KID); err != nil {
		return fmt.Errorf("invalid KID : %w", err)
	}

	if p.Sig == nil {
		return fmt.Errorf("missing signature")
	}
	// Check the signature decode results in a success or not?
	_, err := tpm2.DecodeSignature(bytes.NewBuffer(*p.Sig))
	if err != nil {
		return fmt.Errorf("not a valid signature")
	}

	if p.AttestInfo == nil {
		return fmt.Errorf("missing attestation data")
	}
	_, err = tpm2.DecodeAttestationData(*p.AttestInfo)
	if err != nil {
		return fmt.Errorf("unable to decode attestation information")
	}

	return nil
}

// HashAlgID represents a IANA Supported Hash Algorithms
const (
	UnSupportedAlg = 0
)

// PCRInfo contains a slice of PCR indexes and a hash algorithm used in
// them.
type PCRInfo struct {
	HashAlgID uint64
	PCRs      []int
}

type PCRDetails struct {
	PCRinfo   PCRInfo
	PCRDigest []byte
}

type AttestationInfo struct {
	Magic uint32
	Type  uint16
	Nonce []byte
	PCR   PCRDetails
}

// GetAttestationInfo only decodes relevant information from TPM2 library and sets
// in the returned structure correctly
func (p PAT) GetAttestationInfo() (*AttestationInfo, error) {
	attInfo := &AttestationInfo{}
	if p.AttestInfo == nil {
		return nil, fmt.Errorf("no attest information")
	}
	ad, err := tpm2.DecodeAttestationData(*p.AttestInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to decode supplied attestation information %w", err)
	}
	attInfo.Magic = ad.Magic
	attInfo.Type = uint16(ad.Type)

	if ad.AttestedQuoteInfo == nil {
		return nil, fmt.Errorf("no quote information in the attestInfo")
	}

	hashAlgID := tpmHashAlgToSWIDHash(ad.AttestedQuoteInfo.PCRSelection.Hash)
	if hashAlgID == UnSupportedAlg {
		return nil, fmt.Errorf("unable to map the attestation algorithm")

	}
	attInfo.PCR.PCRinfo.HashAlgID = hashAlgID

	attInfo.PCR.PCRinfo.PCRs = ad.AttestedQuoteInfo.PCRSelection.PCRs

	buf := ad.AttestedQuoteInfo.PCRDigest
	attInfo.PCR.PCRDigest = buf
	// Set the Nonce, it is also U16bytes, needs to process it further!
	attInfo.Nonce = ad.ExtraData
	return attInfo, nil
}

// Verify Verifies the Signature on the given platform attestation token
// using supplied Public Key
func (p PAT) Verify(key crypto.PublicKey) error {

	if p.AttestInfo == nil || len(*p.AttestInfo) == 0 {
		return fmt.Errorf("no payload content to verify")
	}

	if p.Sig == nil || len(*p.Sig) == 0 {
		return fmt.Errorf("no signature on the platform token")
	}
	err := verify(key, *p.AttestInfo, *p.Sig)
	if err != nil {
		return fmt.Errorf("failed to verify the signature %w", err)
	}
	return nil
}

func (p *PAT) EncodeAttestationInfo(attInfo *AttestationInfo) error {
	ad := tpm2.AttestationData{}
	if attInfo == nil {
		return fmt.Errorf("no attestation information supplied")
	}
	setTpmAttestDefaults(&ad)
	ad.Magic = attInfo.Magic
	ad.ExtraData = attInfo.Nonce
	ad.Type = tpm2.TagAttestQuote
	q := &tpm2.QuoteInfo{}
	q.PCRSelection.Hash = swidHashAlgToTPMAlg(attInfo.PCR.PCRinfo.HashAlgID)
	q.PCRSelection.PCRs = attInfo.PCR.PCRinfo.PCRs
	q.PCRDigest = attInfo.PCR.PCRDigest
	ad.AttestedQuoteInfo = q

	adbuf, err := ad.Encode()
	if err != nil {
		return fmt.Errorf("unable to encode the attestation information")
	}
	p.AttestInfo = &adbuf
	return nil
}
