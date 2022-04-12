package aws

type AWSController struct {
	XtraDBClusterManager *XtraDBClusterManager
}

func (c *AWSController) Validate() bool {
	if c == nil {
		return false
	}

	return true
}
