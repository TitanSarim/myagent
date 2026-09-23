package security

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		argv []string
		want Class
	}{
		{[]string{"go", "test", "./..."}, ClassAuto},
		{[]string{"git", "status"}, ClassAuto},
		{[]string{"git", "push", "--force"}, ClassBlocked},
		{[]string{"sudo", "ls"}, ClassBlocked},
		{[]string{"rm", "-rf", "foo"}, ClassBlocked},
		{[]string{"npm", "install"}, ClassApproval},
		{[]string{"docker", "ps"}, ClassApproval},
	}
	for _, tc := range cases {
		got := ClassifyCommand(tc.argv)
		if got != tc.want {
			t.Errorf("%v => %s want %s", tc.argv, got, tc.want)
		}
	}
}
