/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package secretsmanager

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awssm "github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	esv1alpha1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1alpha1"
	fakesm "github.com/external-secrets/external-secrets/pkg/provider/aws/secretsmanager/fake"
	sess "github.com/external-secrets/external-secrets/pkg/provider/aws/session"
)

func TestConstructor(t *testing.T) {
	s, err := sess.New("1111", "2222", "foo", "", nil)
	assert.Nil(t, err)
	c, err := New(s)
	assert.Nil(t, err)
	assert.NotNil(t, c.client)
}

// test the sm<->aws interface
// make sure correct values are passed and errors are handled accordingly.
func TestGetSecret(t *testing.T) {
	fake := &fakesm.Client{}
	p := &SecretsManager{
		client: fake,
	}
	for i, row := range []struct {
		apiInput       *awssm.GetSecretValueInput
		apiOutput      *awssm.GetSecretValueOutput
		rr             esv1alpha1.ExternalSecretDataRemoteRef
		apiErr         error
		expectError    string
		expectedSecret string
	}{
		{
			// good case: default version is set
			// key is passed in, output is sent back
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key: "/baz",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String("RRRRR"),
			},
			apiErr:         nil,
			expectError:    "",
			expectedSecret: "RRRRR",
		},
		{
			// good case: extract property
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key:      "/baz",
				Property: "/shmoo",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String(`{"/shmoo": "bang"}`),
			},
			apiErr:         nil,
			expectError:    "",
			expectedSecret: "bang",
		},
		{
			// bad case: missing property
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key:      "/baz",
				Property: "INVALPROP",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String(`{"/shmoo": "bang"}`),
			},
			apiErr:         nil,
			expectError:    "key INVALPROP does not exist in secret",
			expectedSecret: "",
		},
		{
			// bad case: extract property failure due to invalid json
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key:      "/baz",
				Property: "INVALPROP",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String(`------`),
			},
			apiErr:         nil,
			expectError:    "key INVALPROP does not exist in secret",
			expectedSecret: "",
		},
		{
			// case: ssm.SecretString may be nil but binary is set
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key: "/baz",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: nil,
				SecretBinary: []byte("yesplease"),
			},
			apiErr:         nil,
			expectError:    "",
			expectedSecret: "yesplease",
		},
		{
			// case: both .SecretString and .SecretBinary is nil
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key: "/baz",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: nil,
				SecretBinary: nil,
			},
			apiErr:         nil,
			expectError:    "no secret string nor binary for key",
			expectedSecret: "",
		},
		{
			// case: secretOut.SecretBinary JSON parsing
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key:      "/baz",
				Property: "foobar.baz",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: nil,
				SecretBinary: []byte(`{"foobar":{"baz":"nestedval"}}`),
			},
			apiErr:         nil,
			expectError:    "",
			expectedSecret: "nestedval",
		},
		{
			// should pass version
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/foo/bar"),
				VersionStage: aws.String("1234"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key:     "/foo/bar",
				Version: "1234",
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String("FOOBA!"),
			},
			apiErr:         nil,
			expectError:    "",
			expectedSecret: "FOOBA!",
		},
		{
			// should return err
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/foo/bar"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key: "/foo/bar",
			},
			apiOutput:   &awssm.GetSecretValueOutput{},
			apiErr:      fmt.Errorf("oh no"),
			expectError: "oh no",
		},
	} {
		fake.WithValue(row.apiInput, row.apiOutput, row.apiErr)
		out, err := p.GetSecret(context.Background(), row.rr)
		if !ErrorContains(err, row.expectError) {
			t.Errorf("[%d] unexpected error: %s, expected: '%s'", i, err.Error(), row.expectError)
		}
		if string(out) != row.expectedSecret {
			t.Errorf("[%d] unexpected secret: expected %s, got %s", i, row.expectedSecret, string(out))
		}
	}
}

func TestGetSecretMap(t *testing.T) {
	fake := &fakesm.Client{}
	p := &SecretsManager{
		client: fake,
	}
	for i, row := range []struct {
		apiInput     *awssm.GetSecretValueInput
		apiOutput    *awssm.GetSecretValueOutput
		rr           esv1alpha1.ExternalSecretDataRemoteRef
		expectedData map[string]string
		apiErr       error
		expectError  string
	}{
		{
			// good case: default version & deserialization
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String(`{"foo":"bar"}`),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key: "/baz",
			},
			expectedData: map[string]string{
				"foo": "bar",
			},
			apiErr:      nil,
			expectError: "",
		},
		{
			// bad case: api error returned
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String(`{"foo":"bar"}`),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key: "/baz",
			},
			expectedData: map[string]string{
				"foo": "bar",
			},
			apiErr:      fmt.Errorf("some api err"),
			expectError: "some api err",
		},
		{
			// bad case: invalid json
			apiInput: &awssm.GetSecretValueInput{
				SecretId:     aws.String("/baz"),
				VersionStage: aws.String("AWSCURRENT"),
			},
			apiOutput: &awssm.GetSecretValueOutput{
				SecretString: aws.String(`-----------------`),
			},
			rr: esv1alpha1.ExternalSecretDataRemoteRef{
				Key: "/baz",
			},
			expectedData: map[string]string{},
			apiErr:       nil,
			expectError:  "unable to unmarshal secret",
		},
	} {
		fake.WithValue(row.apiInput, row.apiOutput, row.apiErr)
		out, err := p.GetSecretMap(context.Background(), row.rr)
		if !ErrorContains(err, row.expectError) {
			t.Errorf("[%d] unexpected error: %s, expected: '%s'", i, err.Error(), row.expectError)
		}
		if cmp.Equal(out, row.expectedData) {
			t.Errorf("[%d] unexpected secret data: expected %#v, got %#v", i, row.expectedData, out)
		}
	}
}

func ErrorContains(out error, want string) bool {
	if out == nil {
		return want == ""
	}
	if want == "" {
		return false
	}
	return strings.Contains(out.Error(), want)
}
