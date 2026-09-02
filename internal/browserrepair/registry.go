package browserrepair

type rootSpec struct {
	Family Family
	Path   string
}

var roots = []rootSpec{
	{Chromium, ".config/chromium"}, {Chromium, ".config/ungoogled-chromium"},
	{Chromium, ".config/google-chrome"}, {Chromium, ".config/google-chrome-beta"},
	{Chromium, ".config/google-chrome-unstable"}, {Chromium, ".config/BraveSoftware/Brave-Browser"},
	{Chromium, ".config/BraveSoftware/Brave-Browser-Beta"}, {Chromium, ".config/BraveSoftware/Brave-Browser-Nightly"},
	{Chromium, ".config/microsoft-edge"}, {Chromium, ".config/microsoft-edge-beta"},
	{Chromium, ".config/microsoft-edge-dev"}, {Chromium, ".config/vivaldi"},
	{Chromium, ".config/vivaldi-snapshot"}, {Chromium, ".config/opera"},
	{Chromium, ".config/opera-beta"}, {Chromium, ".config/opera-developer"},
	{Chromium, ".config/thorium"}, {Chromium, ".config/slimjet"},
	{Chromium, ".config/yandex-browser"},
	{Mozilla, ".mozilla/firefox"}, {Mozilla, ".mozilla/firefox-esr"},
	{Mozilla, ".librewolf"}, {Mozilla, ".waterfox"}, {Mozilla, ".floorp"},
	{Mozilla, ".zen"}, {Mozilla, ".mullvad-browser"}, {Mozilla, ".tor-browser"},
	{Chromium, ".var/app/org.chromium.Chromium/config/chromium"},
	{Chromium, ".var/app/com.google.Chrome/config/google-chrome"},
	{Chromium, ".var/app/com.brave.Browser/config/BraveSoftware/Brave-Browser"},
	{Chromium, ".var/app/com.microsoft.Edge/config/microsoft-edge"},
	{Chromium, ".var/app/com.vivaldi.Vivaldi/config/vivaldi"},
	{Chromium, ".var/app/com.opera.Opera/config/opera"},
	{Mozilla, ".var/app/org.mozilla.firefox/.mozilla/firefox"},
	{Mozilla, ".var/app/io.gitlab.librewolf-community/.librewolf"},
	{Mozilla, ".var/app/net.waterfox.waterfox/.waterfox"},
	{Mozilla, ".var/app/one.ablaze.floorp/.floorp"},
	{Mozilla, ".var/app/app.zen_browser.zen/.zen"},
	{Mozilla, ".var/app/net.mullvad.MullvadBrowser/.mullvad-browser"},
}

var chromiumLocks = []string{"SingletonCookie", "SingletonLock", "SingletonSocket"}
var mozillaLocks = []string{".parentlock", "lock"}
