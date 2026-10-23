package win11menu

// Menú clásico («Mostrar más opciones») con el mismo servidor COM.
//
// Ojo: un verbo con ExplorerCommandHandler solo funciona con DLL (el shell lo
// crea dentro del proceso), así que aquí se usa DelegateExecute, que sí admite
// servidores .exe. Cada verbo de registro queda así:
//
//	HKCU\Software\Classes\*\shell\TurboMENU         MUIVerb="Turbo", SubCommands=""
//	   \shell\reducir                               MUIVerb="Reducir tamaño", Icon=…, AppliesTo=…, MultiSelectModel="Player"
//	      \command                                  DelegateExecute=CLSIDExecute
//
// El Explorador llama a Serve con acción = nombre de la clave («reducir») y todas
// las rutas seleccionadas en una sola llamada (no un proceso por archivo).

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

func clsidKey(g string) string { return `Software\Classes\CLSID\` + g }

// RegisterExecuteServer declara en HKCU el .exe como servidor de CLSIDExecute.
func RegisterExecuteServer(exePath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, clsidKey(CLSIDExecute.String())+`\LocalServer32`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("", fmt.Sprintf(`"%s" %s`, exePath, ServerArg))
}

// UnregisterExecuteServer borra lo que creó RegisterExecuteServer.
func UnregisterExecuteServer() error {
	err := DeleteKeyTree(registry.CURRENT_USER, clsidKey(CLSIDExecute.String()))
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

// DelegateExecuteValue es el valor para command\DelegateExecute.
func DelegateExecuteValue() string { return CLSIDExecute.String() }

// DeleteKeyTree borra una clave con todas sus subclaves.
func DeleteKeyTree(root registry.Key, path string) error {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	subs, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, s := range subs {
		if err := DeleteKeyTree(root, path+`\`+s); err != nil {
			return err
		}
	}
	return registry.DeleteKey(root, path)
}
