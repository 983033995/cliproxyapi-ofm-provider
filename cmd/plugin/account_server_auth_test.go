package main

import (
 "testing"
)

func TestServerDeviceLoginOnlyForCompatibleFlows(t *testing.T) {
 for _, id := range []string{"qoder","qodercn","cline","minimax"} {
  if !serverDeviceLoginProviders[id] { t.Errorf("expected server-compatible device authorization for %s",id) }
 }
 for _, id := range []string{"lobsterai","trae","loomy","raccoon","gemini","zcode","workbuddy","codearts"} {
  if serverDeviceLoginProviders[id] { t.Errorf("server callback/login compatibility unverified: %s",id) }
 }
}
func TestSafeRemoteLoginURL(t *testing.T) {
 good:=[]string{
  "https://accounts.example.com/oauth/device?state=abc",
  "https://auth.example.com/verify?user_code=111",
 }
 for _, u:=range good {
  if !safeRemoteLoginURL(u){t.Errorf("rejects external HTTPS URL %q",u)}
 }
 bad:=[]string{
  "http://auth.example.com/verify",
  "http://127.0.0.1:18080/authorize",
  "https://127.0.0.1:18080/authorize",
  "https://localhost/verify",
  "https://some.local/verify",
  "https://[::1]:8443/verify",
  "https://foo:password@example.com/verify",
  "javascript:alert(1)",
  "https://example.com/verify#local",
 }
 for _, u:=range bad {
  if safeRemoteLoginURL(u){t.Errorf("unsafe login URL accepted: %q",u)}
 }
}
func TestServerIDsAndWhitelist(t *testing.T) {
 if _,ok:=accountChannelID("qodercn/qfmodel");!ok {t.Fatal("Qoder CN channel must map")}
 for _,id:=range []string{"qoder-abc12","cline_123","minimax-999"}{
  if !validAccountID(id){t.Errorf("valid account identifier rejected %q",id)}
 }
 for _,id:=range []string{"../bad","abc/token","a","bad key", "zz.zz"}{
  if validAccountID(id){t.Errorf("unsafe account identifier accepted %q",id)}
 }
}
