/**
 * @typedef {Object} KeySwift
 * @property {function(): string} getActiveWindowClass
 * @property {function(): [string]} getPressedKeys
 * @property {function(): "up" | "down"} getKeyState
 * @property {function([string]): void} sendKeys
 * @property {function([string], function(): void): void} onKeyPress
 *
 * @typedef {Object} globalThis
 */


// KeySwift script for key mapping

const Terminals = ["kitty"];

const curWindowClass = KeySwift.getActiveWindowClass();
const inTerminal = Terminals.includes(curWindowClass);

KeySwift.onKeyPress(["cmd", "c"], () => {
    if (inTerminal) {
        KeySwift.sendKeys(["ctrl", "shift", "c"]);
    } else {
        KeySwift.sendKeys(["cmd", "c"]);
    }
});

KeySwift.onKeyPress(["cmd", "v"], () => {
    if (inTerminal) {
        KeySwift.sendKeys(["cmd", "shift", "v"]);
    } else {
        KeySwift.sendKeys(["cmd", "v"]);
    }
});

KeySwift.onKeyPress(["cmd", "x"], () => {
    if (inTerminal) {
        // no need to handle
        KeySwift.sendKeys(["cmd", "x"]);
    } else {
        KeySwift.sendKeys(["ctrl", "x"]);
    }
});
