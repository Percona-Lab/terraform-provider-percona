package utils

import (
	"io"

	"github.com/go-ini/ini"
	"github.com/pkg/errors"
)

func SetIniFields(section string, keysAndValues map[string]string) func(r io.ReadWriteSeeker) error {
	return func(rws io.ReadWriteSeeker) error {
		data, err := io.ReadAll(rws)
		if err != nil {
			return errors.Wrap(err, "read file")
		}
		iniFile, err := ini.LoadSources(ini.LoadOptions{
			AllowBooleanKeys: true,
		}, data)
		if err != nil {
			return errors.Wrap(err, "load ini file")
		}
		for k, v := range keysAndValues {
			iniFile.Section(section).Key(k).SetValue(v)
		}
		if _, err = rws.Seek(0, io.SeekStart); err != nil {
			return errors.Wrap(err, "seek")
		}
		_, err = iniFile.WriteTo(rws)
		if err != nil {
			return errors.Wrap(err, "write to ini file")
		}
		return nil
	}
}
