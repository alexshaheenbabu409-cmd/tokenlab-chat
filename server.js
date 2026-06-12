const express = require('express');
const cors = require('cors');
const OpenAI = require('openai');
const { config, searchInternet } = require('./config');

const app = express();
const port = process.env.PORT || 8080;

app.use(cors());
app.use(express.json());

const sessions = {};

// এআই-এর টুলবক্স הגדרה (Definition)
const aiTools = [
    {
        type: "function",
        function: {
            name: "search_internet",
            description: "Search the internet for current events, real-time news, live scores, weather, or any information you don't know.",
            parameters: {
                type: "object",
                properties: {
                    query: {
                        type: "string",
                        description: "The highly optimized search query to look up on Google."
                    }
                },
                required: ["query"]
            }
        }
    }
];

app.get('/', (req, res) => {
    res.json({
        status: "TokenLab Chat Engine (AI Tools Mode) is running",
        version: "5.0",
        endpoints: "/chat, /history"
    });
});

app.post('/chat', async (req, res) => {
    const { user_id, message, model } = req.body;
    
    if (!config.apiKey) return res.status(500).json({ error: "❌ TOKENLAB_API_KEY is required" });
    if (!message) return res.status(400).json({ error: "Message is required" });

    const userId = user_id || "default";
    const activeModel = model || config.defaultModel;

    const client = new OpenAI({
        apiKey: config.apiKey,
        baseURL: config.apiBase
    });

    if (!sessions[userId]) sessions[userId] = [];

    const messages = [];
    if (config.systemPrompt) {
        messages.push({ role: "system", content: config.systemPrompt });
    }
    messages.push(...sessions[userId]);
    messages.push({ role: "user", content: message });

    try {
        // ধাপ ১: মডেলের কাছে প্রশ্ন ও টুলবক্স পাঠানো
        let response = await client.chat.completions.create({
            model: activeModel,
            messages: messages,
            tools: aiTools,
            tool_choice: "auto" // মডেল নিজে সিদ্ধান্ত নেবে টুল লাগবে কি না
        });

        let responseMessage = response.choices[0]?.message;

        // ধাপ ২: মডেল যদি বলে "আমার সার্চ টুল লাগবে!"
        if (responseMessage.tool_calls) {
            messages.push(responseMessage); // মডেলের টুল রিকোয়েস্ট হিস্ট্রিতে যোগ করা
            
            for (const toolCall of responseMessage.tool_calls) {
                if (toolCall.function.name === "search_internet") {
                    const args = JSON.parse(toolCall.function.arguments);
                    console.log(`🔍 AI is searching for: ${args.query}`); // লগে দেখতে পাবেন সে কী লিখে সার্চ করছে
                    
                    const searchResult = await searchInternet(args.query);
                    
                    // সার্চ রেজাল্ট মডেলে পাঠানো
                    messages.push({
                        role: "tool",
                        tool_call_id: toolCall.id,
                        content: searchResult
                    });
                }
            }

            // ধাপ ৩: সার্চ রেজাল্ট পাওয়ার পর মডেলকে ফাইনাল উত্তর দিতে বলা
            response = await client.chat.completions.create({
                model: activeModel,
                messages: messages
            });
            
            responseMessage = response.choices[0]?.message;
        }

        const reply = responseMessage?.content || "";

        // সেশন মেমরিতে সেভ করা (শুধু মূল প্রশ্ন ও ফাইনাল উত্তর, হাবিজাবি সার্চ রেজাল্ট নয়)
        sessions[userId].push({ role: "user", content: message });
        sessions[userId].push({ role: "assistant", content: reply });

        if (sessions[userId].length > config.maxHistory * 2) {
            sessions[userId] = sessions[userId].slice(-config.maxHistory * 2);
        }

        res.json({ reply: reply });

    } catch (error) {
        console.error("Tools Execution Error:", error.message);
        res.status(502).json({ error: "Upstream API error during tool execution" });
    }
});

app.get('/history', (req, res) => {
    const userId = req.query.user_id || "default";
    res.json({ history: sessions[userId] || [] });
});

app.listen(port, () => {
    console.log(`🚀 AI Tools Server running on port ${port}`);
});
