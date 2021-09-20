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

package util

import (
	"crypto/tls"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"k8s.io/utils/strings/slices"
)

type AuthConfig struct {
	Password  string
	TLSConfig *tls.Config
}

var (
	redisACLCategoryReg = regexp.MustCompile(`\s[+\-]@(\S+)`)
)

func CheckRule(aclRules string) error {
	matches := redisACLCategoryReg.FindAllStringSubmatch(aclRules, -1)
	allowedGroups := []string{
		"keyspace", "read", "write", "set", "sortedset", "list", "hash", "string",
		"bitmap", "hyperloglog", "geo", "stream", "pubsub", "admin", "fast", "slow",
		"blocking", "dangerous", "connection", "transaction", "scripting", "all",
	}
	subgroups := []string{}
	for _, match := range matches {
		if len(match) > 1 {
			group := match[1]
			if !slices.Contains(allowedGroups, group) {
				return fmt.Errorf("acl rule group %s is not allowed", group)
			}
			if slices.Contains(subgroups, group) {
				return fmt.Errorf("acl rule group %s is duplicated", group)
			}
			subgroups = append(subgroups, group)
		}
	}

	//切分 aclrules
	rules := strings.Split(aclRules, " ")
	for _, rule := range rules {
		if strings.HasPrefix(rule, ">") || strings.HasPrefix(rule, "<") {
			return fmt.Errorf("acl password rule %s is not allowed", rule)
		}
		if slices.Contains([]string{"on", "off", "nopass", "reset", "resetpass"}, rule) {
			return fmt.Errorf("acl rule %s is not allowed", rule)
		}
		if unicode.IsLetter(rune(rule[0])) {
			if !slices.Contains([]string{"on", "off", "nopass", "reset",
				"resetpass", "allcommands", "nocommands", "allkeys", "resetkeys",
				"allchannels", "resetchannels", "clearselectors", "sanitize-payload",
				"skip-sanitize-payload"}, rule) {
				return fmt.Errorf("acl rule %s is not allowed", rule)
			}
		} else {
			//如果 不是以 &+->~% 开头报错
			if !slices.Contains([]string{"&", "+", "-", ">", "<", "~", "%"}, string(rule[0])) {
				return fmt.Errorf("acl rule %s is not allowed", rule)
			}
		}

	}
	return nil
}

func CheckUserRuleUpdate(ruleSource string) error {
	rules := strings.Split(ruleSource, " ")
	invalidRule := ""
	for _, rule := range rules {
		if rule == "+acl" {
			return fmt.Errorf("acl rule %s is not invalid", rule)
		}
		if slices.Contains([]string{"+@slow", "+@all", "+@admin", "+@dangerous"}, rule) {
			invalidRule = rule
		}
		if strings.HasPrefix(rule, "+acl|") {
			invalidRule = rule
		}
		if slices.Contains([]string{"-@slow", "-@all", "-@admin", "-@dangerous", "-acl"}, rule) {
			invalidRule = ""
		}
	}
	if invalidRule != "" {
		return fmt.Errorf("acl rule %s include acl command,need add '-acl' rules", invalidRule)
	}
	return nil
}
