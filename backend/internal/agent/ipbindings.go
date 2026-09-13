// Liaisons IP statiques (ip bindings) routeur.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// buildIpbindingAdd — F7 : /ip hotspot ip-binding add (mac requise, type
// bypassed|blocked, address/comment optionnels).
func (b Builder) buildIpbindingAdd(cmd model.Command) string {
	mac := strings.TrimSpace(plStr(cmd.Payload, "mac"))
	address := strings.TrimSpace(plStr(cmd.Payload, "address"))
	comment := plStr(cmd.Payload, "comment")
	typ := plStr(cmd.Payload, "type")
	if typ != "blocked" {
		typ = "bypassed"
	}
	line := `/ip hotspot ip-binding add mac-address="` + rosEscape(mac) + `" type=` + typ
	if address != "" {
		line += ` address="` + rosEscape(address) + `"`
	}
	if comment != "" {
		line += ` comment="` + rosEscape(comment) + `"`
	}
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + line + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildIpbindingSet — F7 : set [find mac-address=…] disabled=yes|no (+address).

// buildIpbindingSet — F7 : set [find mac-address=…] disabled=yes|no (+address).
func (b Builder) buildIpbindingSet(cmd model.Command) string {
	mac := strings.TrimSpace(plStr(cmd.Payload, "mac"))
	set := `/ip hotspot ip-binding set [find mac-address="` + rosEscape(mac) + `"]`
	if plHas(cmd.Payload, "disabled") {
		if plBool(cmd.Payload, "disabled") {
			set += " disabled=yes"
		} else {
			set += " disabled=no"
		}
	}
	if address := strings.TrimSpace(plStr(cmd.Payload, "address")); address != "" {
		set += ` address="` + rosEscape(address) + `"`
	}
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + set + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildIpbindingRemove — F7 : remove [find mac-address=…].

// buildIpbindingRemove — F7 : remove [find mac-address=…].
func (b Builder) buildIpbindingRemove(cmd model.Command) string {
	mac := strings.TrimSpace(plStr(cmd.Payload, "mac"))
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { /ip hotspot ip-binding remove [find mac-address=\"" + rosEscape(mac) + "\"] } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildReadDhcp — F9 : /ip dhcp-server lease → mac|address|host|expires|status.
