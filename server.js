const express = require('express');
const cors = require('cors');
const OpenAI = require('openai'); 
const { config, searchInternet, shouldSearchQuery } = require('./config');

const app = express();
const port = process.env.PORT || 8080;

app.use(cors());
app.use(express.json());

const sessions = {};

app.get('/', (req, res) => {
    res.json({
        status: "TokenLab Chat Engine (OpenAI SDK Mode) is running",
        version: "4.1",
        endpoints: "/chat, /history"
    });
});

app.post('/chat', async (req, res) => {
    const { user_id, message, model, stream } = req.body;
    
    if (!config.apiKey) {
        return res.status(500).json({ error: "❌ TOKENLAB_API_KEY environment variable is required" });
    }
    if (!message) {
        return res.status(400).json({ error: "Message is required" });
    }

    const userId = user_id || "default";
    const activeModel = model || config.defaultModel;

    // OpenAI SDK v4 ইনিশিয়ালাইজেশন
    const client = new OpenAI({
        apiKey: config.apiKey,
        baseURL: config.apiBase
    });

    if (!sessions[userId]) {
        sessions[userId] = [];
    }

    let userPrompt = message;

    if (shouldSearchQuery(message)) {
        const searchResult = await searchInternet(message);
        if (searchResult) {
            userPrompt = `${searchResult}\nUser Question: ${message}`;
        }
    }

    const messages = [];
    if (config.systemPrompt) {
        messages.push({ role: "system", content: config.systemPrompt });
    }
    messages.push(...sessions[userId]);
    messages.push({ role: "user", content: userPrompt });

    try {
        const response = await client.chat.completions.create({
            model: activeModel,
            messages: messages,
            stream: stream || false
        });

        const reply = response.choices[0]?.message?.content || "";

        sessions[userId].push({ role: "user", content: message });
        sessions[userId].push({ role: "assistant", content: reply });

        if (sessions[userId].length > config.maxHistory * 2) {
            sessions[userId] = sessions[userId].slice(-config.maxHistory * 2);
        }

        res.json({ reply: reply });

    } catch (error) {
        console.error("OpenAI SDK Error:", error.message);
        res.status(502).json({ error: "Upstream API error via SDK" });
    }
});

app.get('/history', (req, res) => {
    const userId = req.query.user_id || "default";
    res.json({ history: sessions[userId] || [] });
});

app.listen(port, () => {
    console.log(`🚀 Server running with OpenAI SDK on port ${port}`);
});
             
