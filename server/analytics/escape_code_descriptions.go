package analytics

import (
	"strings"
)

// DEC Private Mode descriptions
var decPrivateModes = map[string]string{
	// Cursor modes
	"1":    "Application Cursor Keys (DECCKM)",
	"2":    "ANSI/VT52 Mode (DECANM)",
	"3":    "132 Column Mode (DECCOLM)",
	"4":    "Smooth Scroll (DECSCLM)",
	"5":    "Reverse Video (DECSCNM)",
	"6":    "Origin Mode (DECOM)",
	"7":    "Auto-Wrap Mode (DECAWM)",
	"8":    "Auto-Repeat Keys (DECARM)",
	"9":    "X10 Mouse Reporting",
	"12":   "Cursor Blink (att610)",
	"25":   "Cursor Visibility (DECTCEM)",

	// Alternate screen buffer
	"47":   "Alternate Screen Buffer (old)",
	"1047": "Alternate Screen Buffer",
	"1048": "Save/Restore Cursor (xterm)",
	"1049": "Alternate Screen Buffer with Cursor Save",

	// Mouse tracking
	"1000": "X11 Mouse Reporting (Normal)",
	"1001": "Highlight Mouse Tracking",
	"1002": "Cell Motion Mouse Tracking",
	"1003": "All Motion Mouse Tracking",
	"1004": "Focus In/Out Events",
	"1005": "UTF-8 Mouse Encoding",
	"1006": "SGR Mouse Encoding",
	"1007": "Alternate Scroll Mode",
	"1015": "urxvt Mouse Encoding",
	"1016": "SGR-Pixels Mouse Encoding",

	// Bracketed paste
	"2004": "Bracketed Paste Mode",

	// Synchronous updates
	"2026": "Synchronous Update Mode",

	// Other xterm modes
	"1034": "Meta Key Sends Escape",
	"1035": "Special Modifiers",
	"1036": "Meta Key Sends ESC",
	"1037": "Delete/Backspace Keys",
	"1039": "Alt Key Sends Escape",
	"1040": "Keep Selection",
	"1041": "Select to Clipboard",
	"1042": "Bell Urgency",
	"1043": "Raise on Bell",
	"1044": "Keep Clipboard",
	"1046": "Enable Alternate Scroll",
	"1050": "Function Key Mode",
	"1051": "Sun Function Keys",
	"1052": "HP Function Keys",
	"1053": "SCO Function Keys",
	"1060": "Legacy Keyboard",
	"1061": "VT220 Keyboard",
	"2001": "Save Cursor to Stack",
	"2002": "Restore Cursor from Stack",
	"2003": "Set Title Modes",
}

// GetDECPrivateModeDescription returns a human-readable description for a DEC private mode
func GetDECPrivateModeDescription(mode string) string {
	if desc, ok := decPrivateModes[mode]; ok {
		return desc
	}
	return ""
}

// OSC command descriptions
var oscCommands = map[string]string{
	"0":   "Set Icon Name and Window Title",
	"1":   "Set Icon Name",
	"2":   "Set Window Title",
	"3":   "Set X Property",
	"4":   "Change/Query Color",
	"5":   "Change Special Color",
	"6":   "Enable/Disable Special Colors",
	"7":   "Set Current Directory",
	"8":   "Hyperlink",
	"9":   "Desktop Notification (iTerm2)",
	"10":  "Set Foreground Color",
	"11":  "Set Background Color",
	"12":  "Set Cursor Color",
	"13":  "Set Mouse Foreground",
	"14":  "Set Mouse Background",
	"15":  "Set Tektronix Foreground",
	"16":  "Set Tektronix Background",
	"17":  "Set Highlight Color",
	"18":  "Set Tektronix Cursor",
	"19":  "Set Highlight Foreground",
	"46":  "Set Log File",
	"50":  "Set Font",
	"51":  "Reserved",
	"52":  "Clipboard Operation",
	"104": "Reset Color",
	"105": "Reset Special Color",
	"106": "Enable/Disable Special Color",
	"110": "Reset Foreground Color",
	"111": "Reset Background Color",
	"112": "Reset Cursor Color",
	"113": "Reset Mouse Foreground",
	"114": "Reset Mouse Background",
	"115": "Reset Tektronix Foreground",
	"116": "Reset Tektronix Background",
	"117": "Reset Highlight Color",
	"118": "Reset Tektronix Cursor",
	"119": "Reset Highlight Foreground",
	"133": "Shell Integration (prompt start/end)",
	"777": "rxvt Notification",
	"1337": "iTerm2 Proprietary",
}

// GetOSCDescription returns a human-readable description for an OSC command
func GetOSCDescription(cmd string) string {
	if desc, ok := oscCommands[cmd]; ok {
		return "OSC: " + desc
	}
	return "OSC Command " + cmd
}

// Simple escape sequence descriptions
var simpleEscapes = map[byte]string{
	'7': "Save Cursor (DECSC)",
	'8': "Restore Cursor (DECRC)",
	'D': "Index (IND) - Cursor down, scroll if at bottom",
	'E': "Next Line (NEL) - Cursor to start of next line",
	'H': "Tab Set (HTS)",
	'M': "Reverse Index (RI) - Cursor up, scroll if at top",
	'N': "Single Shift G2 (SS2)",
	'O': "Single Shift G3 (SS3)",
	'Z': "Device Attributes Request (DECID)",
	'c': "Reset (RIS) - Full reset",
	'=': "Application Keypad (DECKPAM)",
	'>': "Normal Keypad (DECKPNM)",
}

// DescribeSimpleEscape returns a description for a simple 2-byte escape
func DescribeSimpleEscape(secondByte byte) string {
	if desc, ok := simpleEscapes[secondByte]; ok {
		return desc
	}
	return "Escape " + string(secondByte)
}

// SGR (Select Graphic Rendition) attribute descriptions
var sgrAttributes = map[string]string{
	"0":  "Reset All Attributes",
	"1":  "Bold",
	"2":  "Dim",
	"3":  "Italic",
	"4":  "Underline",
	"5":  "Slow Blink",
	"6":  "Rapid Blink",
	"7":  "Reverse Video",
	"8":  "Hidden",
	"9":  "Strikethrough",
	"21": "Double Underline",
	"22": "Normal Intensity",
	"23": "Not Italic",
	"24": "Not Underlined",
	"25": "Not Blinking",
	"27": "Not Reversed",
	"28": "Not Hidden",
	"29": "Not Strikethrough",
	// Foreground colors
	"30": "Foreground Black",
	"31": "Foreground Red",
	"32": "Foreground Green",
	"33": "Foreground Yellow",
	"34": "Foreground Blue",
	"35": "Foreground Magenta",
	"36": "Foreground Cyan",
	"37": "Foreground White",
	"38": "Set Foreground Color (256/RGB)",
	"39": "Default Foreground",
	// Background colors
	"40": "Background Black",
	"41": "Background Red",
	"42": "Background Green",
	"43": "Background Yellow",
	"44": "Background Blue",
	"45": "Background Magenta",
	"46": "Background Cyan",
	"47": "Background White",
	"48": "Set Background Color (256/RGB)",
	"49": "Default Background",
	// Bright foreground colors
	"90":  "Bright Foreground Black",
	"91":  "Bright Foreground Red",
	"92":  "Bright Foreground Green",
	"93":  "Bright Foreground Yellow",
	"94":  "Bright Foreground Blue",
	"95":  "Bright Foreground Magenta",
	"96":  "Bright Foreground Cyan",
	"97":  "Bright Foreground White",
	"100": "Bright Background Black",
	"101": "Bright Background Red",
	"102": "Bright Background Green",
	"103": "Bright Background Yellow",
	"104": "Bright Background Blue",
	"105": "Bright Background Magenta",
	"106": "Bright Background Cyan",
	"107": "Bright Background White",
}

// DescribeSGR returns a human-readable description of SGR parameters
func DescribeSGR(params string) string {
	if params == "" || params == "0" {
		return "Reset Attributes"
	}

	parts := strings.Split(params, ";")
	if len(parts) == 1 {
		if desc, ok := sgrAttributes[params]; ok {
			return desc
		}
		return "SGR " + params
	}

	// For complex SGR, just summarize
	var descriptions []string
	i := 0
	for i < len(parts) {
		p := parts[i]
		if p == "38" || p == "48" {
			// Extended color - skip the whole sequence
			prefix := "Foreground"
			if p == "48" {
				prefix = "Background"
			}
			if i+1 < len(parts) {
				if parts[i+1] == "5" && i+2 < len(parts) {
					descriptions = append(descriptions, prefix+" 256-Color "+parts[i+2])
					i += 3
					continue
				}
				if parts[i+1] == "2" && i+4 < len(parts) {
					descriptions = append(descriptions, prefix+" RGB")
					i += 5
					continue
				}
			}
		}
		if desc, ok := sgrAttributes[p]; ok {
			descriptions = append(descriptions, desc)
		}
		i++
	}

	if len(descriptions) == 0 {
		return "SGR (" + params + ")"
	}
	if len(descriptions) == 1 {
		return descriptions[0]
	}
	if len(descriptions) <= 3 {
		return strings.Join(descriptions, ", ")
	}
	return descriptions[0] + " + " + string(rune(len(descriptions)-1)) + " more"
}

// Erase in Display descriptions
func GetEraseInDisplayDescription(params string) string {
	switch params {
	case "", "0":
		return "Erase Below"
	case "1":
		return "Erase Above"
	case "2":
		return "Erase All"
	case "3":
		return "Erase Saved Lines (scrollback)"
	default:
		return "Erase Display (" + params + ")"
	}
}

// Erase in Line descriptions
func GetEraseInLineDescription(params string) string {
	switch params {
	case "", "0":
		return "Erase to End of Line"
	case "1":
		return "Erase to Start of Line"
	case "2":
		return "Erase Entire Line"
	default:
		return "Erase Line (" + params + ")"
	}
}
