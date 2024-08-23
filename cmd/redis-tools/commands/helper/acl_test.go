package helper

import (
	"reflect"
	"testing"

	"github.com/alauda/redis-operator/pkg/types/user"
)

func Test_formatACLSetCommand(t *testing.T) {
	type args struct {
		user *user.User
	}
	tests := []struct {
		name     string
		args     args
		wantArgs []string
	}{
		{
			name: "default",
			args: args{
				user: &user.User{
					Name: "default",
					Role: user.RoleDeveloper,
					Rules: []*user.Rule{
						{
							DisallowedCommands: []string{"flushall", "flushdb"},
						},
					},
				},
			},
			wantArgs: []string{"user", "default", "-flushall", "-flushdb", "nopass", "on"},
		},
		{
			name: "custom1",
			args: args{
				user: &user.User{
					Name: "custom1",
					Role: user.RoleDeveloper,
					Rules: []*user.Rule{
						{
							Categories:           []string{"read"},
							DisallowedCategories: []string{"all"},
							DisallowedCommands:   []string{"keys"},
							KeyPatterns:          []string{"*"},
						},
					},
				},
			},
			wantArgs: []string{"user", "custom1", "-@all", "+@read", "-keys", "~*", "nopass", "on"},
		},
		{
			name: "custom2",
			args: args{
				user: &user.User{
					Name: "custom2",
					Role: user.RoleDeveloper,
					Rules: []*user.Rule{
						{
							AllowedCommands:    []string{"cluster"},
							DisallowedCommands: []string{"cluster|setslot", "cluster|nodes"},
							KeyPatterns:        []string{"*"},
						},
					},
				},
			},
			wantArgs: []string{"user", "custom2", "+cluster", "-cluster|setslot", "-cluster|nodes", "~*", "nopass", "on"},
		},
		{
			name: "custom3",
			args: args{
				user: &user.User{
					Name: "custom3",
					Role: user.RoleDeveloper,
					Rules: []*user.Rule{
						{
							DisallowedCommands: []string{"cluster|setslot", "cluster|nodes"},
							KeyPatterns:        []string{"*"},
						},
					},
				},
			},
			wantArgs: []string{"user", "custom3", "-cluster|setslot", "-cluster|nodes", "~*", "nopass", "on"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotArgs := formatACLSetCommand(tt.args.user); !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("formatACLSetCommand() = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}
