package controllers

import (
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
)

func appDomainName() string {
	x := ctrl.GetConfigOrDie()
	commonDomain := strings.Split(x.Host, "api")[1]
	return strings.Split(commonDomain, ":")[0]
}
