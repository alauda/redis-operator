/*
Copyright 2023 The RedisOperator Authors.

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

package user

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
)

const (
	// DefaultUserName from reids 6.0, there is a default user named "default"
	// for compatibility, the default user set as RoleDeveloper
	DefaultUserName         = "default"
	DefaultOperatorUserName = "operator"

	// password secret key name
	PasswordSecretKey = "password"
)

// Rule acl rules
//
// This rule supports redis 7.0, which is compatable with 6.0
type Rule struct {
	// Categories
	Categories []string `json:"categories,omitempty"`
	// AllowedCommands supports <command> and <command>|<subcommand>
	AllowedCommands []string `json:"allowedCommands,omitempty"`
	// DisallowedCommands supports <command> and <command>|<subcommand>
	DisallowedCommands []string `json:"disallowedCommands,omitempty"`
	// KeyPatterns support multi patterns, for 7.0 support %R~ and %W~ patterns
	KeyPatterns []string `json:"keyPatterns,omitempty"`
}

func (r *Rule) Validate() error {
	if r == nil {
		return errors.New("nil rule")
	}
	if len(r.Categories) == 0 && len(r.AllowedCommands) == 0 {
		return errors.New("invalid rule, no allowed command")
	}
	if len(r.KeyPatterns) == 0 {
		return errors.New("invalid rule, no key pattern")
	}
	return nil
}

func (r *Rule) String() string {
	return strings.Join(append(append(append(append([]string{}, r.Categories...),
		r.AllowedCommands...), r.DisallowedCommands...), r.KeyPatterns...), " ")
}

// UserRole
type UserRole string

const (
	RoleOperator  = "Operator"
	RoleDeveloper = "Developer"
)

// NewUser
func NewUser(name string, role UserRole, secret *v1.Secret) (*User, error) {
	var (
		err    error
		passwd *Password
	)
	if secret != nil {
		if passwd, err = NewPassword(secret); err != nil {
			return nil, err
		}
	}
	if name == "" {
		name = DefaultUserName
	}

	user := &User{
		Name:     name,
		Role:     role,
		Password: passwd,
		Rules: []*Rule{
			{Categories: []string{"all"}, KeyPatterns: []string{"*"}},
		},
	}
	if err := user.Validate(); err != nil {
		return nil, err
	}
	return user, nil
}

// User
type User struct {
	Name     string    `json:"name"`
	Role     UserRole  `json:"role"`
	Password *Password `json:"password,omitempty"`
	Rules    []*Rule   `json:"rules,omitempty"`
}

var (
	usernameReg = regexp.MustCompile(`^[0-9a-zA-Z-]{0,31}$`)
)

// AppendRule
func (u *User) AppendRule(rules ...*Rule) error {
	if u == nil {
		return nil
	}
	for _, rule := range rules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	u.Rules = append(u.Rules, rules...)
	return nil
}

func (u *User) Validate() error {
	if u == nil {
		return fmt.Errorf("nil user")
	}
	if !usernameReg.MatchString(u.Name) {
		return fmt.Errorf("invalid username which should match ^[0-9a-zA-Z-]{0,31}$")
	}

	if u.Role != RoleOperator && u.Role != RoleDeveloper {
		return fmt.Errorf(`unsupported user role "%s"`, u.Role)
	}
	return nil
}

// String
func (u *User) String() string {
	if u == nil {
		return ""
	}

	vals := []string{u.Name, string(u.Role)}
	for _, rule := range u.Rules {
		vals = append(vals, rule.String())
	}
	return strings.Join(vals, " ")
}

// Password
type Password struct {
	SecretName string `json:"secretName,omitempty"`
	secret     *v1.Secret
	data       string
}

// NewPassword
func NewPassword(secret *v1.Secret) (*Password, error) {
	if secret == nil {
		return nil, nil
	}

	p := Password{}
	if err := p.SetSecret(secret); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *Password) GetSecretName() string {
	if p == nil {
		return ""
	}
	return p.SecretName
}

func (p *Password) SetSecret(secret *v1.Secret) error {
	if p == nil || secret == nil {
		return nil
	}

	if val, ok := secret.Data[PasswordSecretKey]; !ok {
		return fmt.Errorf("missing %s field for secret %s", PasswordSecretKey, secret.Name)
	} else {
		p.SecretName = secret.GetName()
		p.secret = secret
		p.data = string(val)
	}
	return nil
}

func (p *Password) Secret() *v1.Secret {
	if p == nil {
		return nil
	}
	return p.secret
}

// String return password in plaintext
func (p *Password) String() string {
	if p == nil {
		return ""
	}
	return p.data
}
