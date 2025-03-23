/**
 * @typedef {Object} KeySwift
 * @property {function(): string} getActiveWindowClass
 * @property {function([string]): void} sendKeys
 * Note that this function needs to be called at the outermost level, not in callback functions or if statements
 * Otherwise, the script will not work
 * @property {function([string], function(): void): void} onKeyPress
 */


// KeySwift script for key mapping

const Terminals = ["kitty", "Gnome-terminal", "org.gnome.Terminal", "com.mitchellh.ghostty"];
const JetBrains = ["jetbrains-goland", "jetbrains-pycharm"]
const VimModeEnabled = ["Cursor"] + JetBrains

const curWindowClass = KeySwift.getActiveWindowClass();
const inTerminal = Terminals.includes(curWindowClass);
const inVimMode = VimModeEnabled.includes(curWindowClass)
const inJetBrains = JetBrains.includes(curWindowClass)

const chromeChangeTabShortcuts = {
    "cmd,1": ["ctrl", "1"],
    "cmd,2": ["ctrl", "2"],
    "cmd,3": ["ctrl", "3"],
    "cmd,4": ["ctrl", "4"],
    "cmd,5": ["ctrl", "5"],
    "cmd,6": ["ctrl", "6"],
    "cmd,7": ["ctrl", "7"],
    "cmd,8": ["ctrl", "8"],
    "cmd,9": ["ctrl", "9"],
}

const emacsShortcuts = {
    "ctrl,a": ["home"],
    "ctrl,e": ["end"],
    "ctrl,b": ["left"],
    "ctrl,f": ["right"],
    "ctrl,d": ["delete"],
    "ctrl,h": ["backspace"],
}

const macOSLikeShortcuts = {
    "cmd,x": ["ctrl", "x"],
    "cmd,a": ["ctrl", "a"],
    "cmd,z": ["ctrl", "z"],
    "cmd,w": ["ctrl", "w"],
    "cmd,t": ["ctrl", "t"],
    "cmd,f": ["ctrl", "f"],
    "cmd,r": ["ctrl", "r"],
}

const jetBrainsShortcuts = {
    "cmd,1": ["alt", "1"],
    "cmd,2": ["alt", "2"],
    "cmd,3": ["alt", "3"],
    "cmd,w": ["ctrl", "4"],
    "cmd,c": ["ctrl", "insert"],
    "cmd,v": ["shift", "insert"],
}

const sublimeTextShortcuts = {
    "cmd,1": ["alt", "1"],
    "cmd,2": ["alt", "2"],
    "cmd,3": ["alt", "3"],
    "cmd,4": ["alt", "4"],
    "cmd,5": ["alt", "5"],
    "cmd,6": ["alt", "6"],
    "cmd,7": ["alt", "7"],
}

KeySwift.onKeyPress(["cmd", "c"], () => {
	if (curWindowClass === "kitty") {
		return
	}
    if (inTerminal) {
        KeySwift.sendKeys(["ctrl", "shift", "c"]);
    } else {
        if (!inJetBrains) {
            KeySwift.sendKeys(["ctrl", "c"]);
        }
    }
});

KeySwift.onKeyPress(["cmd", "v"], () => {
	if (curWindowClass === "kitty") {
		return
	}
    if (curWindowClass === "com.mitchellh.ghostty") {
        KeySwift.sendKeys(["shift", "ctrl", "v"]);
        return
    }

    if (inTerminal) {
        KeySwift.sendKeys(["cmd", "shift", "v"]);
    } else {
        if (!inJetBrains) {
            KeySwift.sendKeys(["ctrl", "v"]);
        }
    }
});

KeySwift.onKeyPress(["cmd", "w"], () => {
    if (curWindowClass === "Cursor") {
        KeySwift.sendKeys(["ctrl", "4"]);
    }
});

for (const [key, value] of Object.entries(macOSLikeShortcuts)) {
    KeySwift.onKeyPress(key.split(","), () => {
        if (!inTerminal && !inJetBrains) {
            KeySwift.sendKeys(value);
        }
    });
}

for (const [key, value] of Object.entries(chromeChangeTabShortcuts)) {
    KeySwift.onKeyPress(key.split(","), () => {
        if (curWindowClass === "Google-chrome") {
            KeySwift.sendKeys(value);
        }
    });
}

for (const [key, value] of Object.entries(emacsShortcuts)) {
    KeySwift.onKeyPress(key.split(","), () => {
        if (!inTerminal && !inVimMode) {
            KeySwift.sendKeys(value);
        }
    });
}

for (const [key, value] of Object.entries(jetBrainsShortcuts)) {
    KeySwift.onKeyPress(key.split(","), () => {
        if (inJetBrains) {
            KeySwift.sendKeys(value);
        }
    });
}

for (const [key, value] of Object.entries(sublimeTextShortcuts)) {
    KeySwift.onKeyPress(key.split(","), () => {
        if (curWindowClass === "sublime_text") {
            KeySwift.sendKeys(value);
        }
    });
}
