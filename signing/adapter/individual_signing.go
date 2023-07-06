package adapter

import (
	"github.com/opensourceways/app-cla-server/models"
	"github.com/opensourceways/app-cla-server/signing/app"
	"github.com/opensourceways/app-cla-server/signing/domain/dp"
)

func NewIndividualSigningAdapter(s app.IndividualSigningService) *individualSigningAdatper {
	return &individualSigningAdatper{s}
}

type individualSigningAdatper struct {
	s app.IndividualSigningService
}

// Sign
func (adapter *individualSigningAdatper) Sign(linkId string, opt *models.IndividualSigning) models.IModelError {
	cmd, err := adapter.cmdToSignIndividualCLA(linkId, opt)
	if err != nil {
		return toModelError(err)
	}

	if err = adapter.s.Sign(&cmd); err != nil {
		return toModelError(err)
	}

	return nil
}

func (adapter *individualSigningAdatper) cmdToSignIndividualCLA(linkId string, opt *models.IndividualSigning) (
	cmd app.CmdToSignIndividualCLA, err error,
) {
	cmd.Link.Id = linkId
	cmd.Link.CLAId = opt.CLAId
	if cmd.Link.Language, err = dp.NewLanguage(opt.CLALanguage); err != nil {
		return
	}

	if cmd.Rep.Name, err = dp.NewName(opt.Name); err != nil {
		return
	}

	if cmd.Rep.EmailAddr, err = dp.NewEmailAddr(opt.Email); err != nil {
		return
	}

	cmd.AllSingingInfo = opt.Info

	return
}

// Check
func (adapter *individualSigningAdatper) Check(linkId string, email string) (bool, models.IModelError) {
	cmd := app.CmdToCheckSinging{
		LinkId: linkId,
	}

	var err error
	if cmd.EmailAddr, err = dp.NewEmailAddr(email); err != nil {
		return false, toModelError(err)
	}

	v, err := adapter.s.Check(&cmd)
	if err != nil {
		return v, toModelError(err)
	}

	return v, nil

}
