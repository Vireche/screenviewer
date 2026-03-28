// Port that ScreenViewer listens on (configurable)
const SCREENVIEWER_PORT = 8765;
const SCREENVIEWER_HOST = "http://localhost";

// Create context menu for images
chrome.contextMenus.create({
    id: "sendToScreenViewer",
    title: "Send to ScreenViewer",
    contexts: ["image"]
});

// Handle context menu clicks
chrome.contextMenus.onClicked.addListener((info, tab) => {
    if (info.menuItemId === "sendToScreenViewer") {
        sendImageToScreenViewer(info.srcUrl, tab.title);
    }
});

async function sendImageToScreenViewer(imageUrl, tabTitle) {
    try {
        console.log("Sending image to ScreenViewer:", imageUrl);

        // Check if ScreenViewer is running (ping the endpoint)
        try {
            const pingResponse = await fetch(`${SCREENVIEWER_HOST}:${SCREENVIEWER_PORT}/ping`);
            console.log("Ping response status:", pingResponse.status);
            if (!pingResponse.ok && pingResponse.status !== 200) {
                showError("ScreenViewer is not responding. Is it running on port " + SCREENVIEWER_PORT + "?");
                return;
            }
        } catch (pingError) {
            console.error("Ping failed:", pingError);
            showError("ScreenViewer is not responding on port " + SCREENVIEWER_PORT + ". Is it running?");
            return;
        }

        // Fetch the image as a blob
        console.log("Fetching image...");
        const imageResponse = await fetch(imageUrl);
        if (!imageResponse.ok) {
            showError("Failed to fetch image from page: " + imageResponse.statusText);
            return;
        }

        const imageBlob = await imageResponse.blob();
        console.log("Image fetched, size:", imageBlob.size, "type:", imageBlob.type);
        const fileName = extractFileName(imageUrl);

        // Send to ScreenViewer
        const formData = new FormData();
        formData.append("image", imageBlob, fileName);
        formData.append("source", tabTitle);

        console.log("Uploading to ScreenViewer...");
        const uploadResponse = await fetch(`${SCREENVIEWER_HOST}:${SCREENVIEWER_PORT}/upload`, {
            method: "POST",
            body: formData
        });

        console.log("Upload response status:", uploadResponse.status);
        const responseText = await uploadResponse.text();
        console.log("Upload response:", responseText);

        if (uploadResponse.ok) {
            showSuccess("Image sent to ScreenViewer");
            console.log("Successfully sent image to ScreenViewer");
        } else {
            showError("Failed to send image. Status: " + uploadResponse.status);
        }
    } catch (error) {
        console.error("Error sending image to ScreenViewer:", error);
        showError("Error: " + error.message);
    }
}

function extractFileName(url) {
    try {
        const urlObj = new URL(url);
        const path = urlObj.pathname;
        let fileName = path.substring(path.lastIndexOf("/") + 1);

        // Remove query params
        if (fileName.includes("?")) {
            fileName = fileName.substring(0, fileName.indexOf("?"));
        }

        // If no filename, generate one
        if (!fileName || fileName === "") {
            fileName = "image_" + Date.now() + ".jpg";
        }

        return fileName;
    } catch (e) {
        return "image_" + Date.now() + ".jpg";
    }
}

function showSuccess(message) {
    chrome.action.setBadgeText({ text: "✓" });
    chrome.action.setBadgeBackgroundColor({ color: "#4CAF50" });
    setTimeout(() => {
        chrome.action.setBadgeText({ text: "" });
    }, 2000);
}

function showError(message) {
    console.error(message);
    chrome.action.setBadgeText({ text: "✗" });
    chrome.action.setBadgeBackgroundColor({ color: "#F44336" });
    setTimeout(() => {
        chrome.action.setBadgeText({ text: "" });
    }, 3000);
}
