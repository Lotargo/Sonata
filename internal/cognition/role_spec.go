package cognition

func RoleSpecFor(role RuntimeRole) (RoleSpec, bool) {
	for _, spec := range runtimeRoleSpecs {
		if spec.Role == role {
			return spec, true
		}
	}
	return RoleSpec{}, false
}
