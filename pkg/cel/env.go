package cel

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	httpauth "github.com/kyverno/kyverno-http-authorizer/pkg/cel/libs/http"
	"github.com/kyverno/kyverno-http-authorizer/pkg/cel/libs/jwt"

	"github.com/kyverno/kyverno/pkg/cel/libs/http"
	"github.com/kyverno/kyverno/pkg/cel/libs/image"
	"github.com/kyverno/kyverno/pkg/cel/libs/imagedata"

	"k8s.io/apiserver/pkg/cel/library"
)

func NewEnv() (*cel.Env, error) {
	// create new cel env
	return cel.NewEnv(
		// configure env
		cel.HomogeneousAggregateLiterals(),
		cel.EagerlyValidateDeclarations(true),
		cel.DefaultUTCTimeZone(true),
		cel.CrossTypeNumericComparisons(true),
		// register common libs
		cel.OptionalTypes(),
		ext.Bindings(),
		ext.Encoders(),
		ext.Lists(),
		ext.Math(),
		ext.Protos(),
		ext.Sets(),
		ext.Strings(),
		// register kubernetes libs
		library.CIDR(),
		library.Format(),
		library.IP(),
		library.Lists(),
		library.Regex(),
		library.URLs(),
		// register our libs
		jwt.Lib(),
		// register kyverno libs
		image.Lib(),
		imagedata.Lib(),
		http.Lib(),
		httpauth.Lib(),
	)
}
