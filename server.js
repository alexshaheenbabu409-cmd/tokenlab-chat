const express = require('express');
const cors = require('cors');
const OpenAI = require('openai');
const { config, searchInternet } = require('./config');

const app = express();
const port = process.env.PORT || 8080;

app.use(cors());
app.use(express.json());

const sessions = {};

// সার্চ টুল — description এ clearly বলা আছে কখন সার্চ করবে না
const aiTools = [
    {
        type: "function",
        function: {
            name: "search_internet",
            description: `Search the internet ONLY when the user asks about:
- Today's news, current events, live prices (crypto, gold, dollar rate)
- Today's weather
- Something that happened after mid-2024

Do NOT search for:
- General knowledge, history, science, coding, math
- Current time or date (you already know this from the system prompt)
- Things you already know from your training

Use maximum 1 search per question. If first search gives enough info, do NOT search again.`,
            parameters: {
                type: "object",
                properties: {
                    query: {
                        type: "string",
                        description: "A short, precise English search query."
                    }
                },
                required: ["query"]
            }
        }
    }
];

app.get('/', (req, res) => {
    res.json({
        status: "নবিতা AI Backend চলছে",
        version: "6.0",
        endpoints: "/chat, /history"
    });
});

app.post('/chat', async (req, res) => {
    const { user_id, message, model } = req.body;

    if (!config.apiKey) return res.status(500).json({ error: "❌ TOKENLAB_API_KEY দরকার" });
    if (!message) return res.status(400).json({ error: "Message দরকার" });

    const userId = user_id || "default";
    const activeModel = model || config.defaultModel;

    const client = new OpenAI({
        apiKey: config.apiKey,
        baseURL: config.apiBase
    });

    if (!sessions[userId]) sessions[userId] = [];

    // প্রতিটি request এ ঢাকার বর্তমান সময় inject করা
    const now = new Date().toLocaleString('bn-BD', {
        timeZone: 'Asia/Dhaka',
        dateStyle: 'full',
        timeStyle: 'short'
    });

    const messages = [];
    if (config.systemPrompt) {
        const systemWithTime = `বর্তমান তারিখ ও সময় (ঢাকা): ${now}\n\n${config.systemPrompt}`;
        messages.push({ role: "system", content: systemWithTime });
    }
    messages.push(...sessions[userId]);
    messages.push({ role: "user", content: message });

    try {
        // ধাপ ১: মডেলের কাছে প্রশ্ন পাঠানো
        let response = await client.chat.completions.create({
            model: activeModel,
            messages: messages,
            tools: aiTools,
            tool_choice: "auto"
        });

        let responseMessage = response.choices[0]?.message;

        // ধাপ ২: সার্চ দরকার হলে — সর্বোচ্চ ২টি call
        if (responseMessage.tool_calls) {
            messages.push(responseMessage);

            let searchCount = 0;
            const MAX_SEARCHES = 2;

            for (const toolCall of responseMessage.tool_calls) {
                if (toolCall.function.name === "search_internet") {
                    // ২টার বেশি সার্চ হলে বাকিগুলো skip
                    if (searchCount >= MAX_SEARCHES) {
                        messages.push({
                            role: "tool",
                            tool_call_id: toolCall.id,
                            content: "Search limit reached. Use your existing knowledge to answer."
                        });
                        continue;
                    }

                    const args = JSON.parse(toolCall.function.arguments);
                    console.log(`🔍 AI is searching for: ${args.query}`);
                    searchCount++;

                    // ১৫ সেকেন্ড timeout
                    let searchResult;
                    try {
                        searchResult = await Promise.race([
                            searchInternet(args.query),
                            new Promise((_, reject) =>
                                setTimeout(() => reject(new Error('Search timeout')), 15000)
                            )
                        ]);
                    } catch (searchError) {
                        console.warn(`⚠️ Search failed: ${searchError.message}`);
                        searchResult = "সার্চ করতে সমস্যা হয়েছে। নিজের জ্ঞান থেকে সেরা উত্তর দাও।";
                    }

                    messages.push({
                        role: "tool",
                        tool_call_id: toolCall.id,
                        content: searchResult
                    });
                }
            }

            // ধাপ ৩: সার্চ রেজাল্ট দিয়ে ফাইনাল উত্তর
            response = await client.chat.completions.create({
                model: activeModel,
                messages: messages
            });

            responseMessage = response.choices[0]?.message;
        }

        const reply = responseMessage?.content || "";

        // সেশন মেমরি — শুধু প্রশ্ন ও উত্তর রাখা
        sessions[userId].push({ role: "user", content: message });
        sessions[userId].push({ role: "assistant", content: reply });

        // পুরনো history ছাঁটা
        if (sessions[userId].length > config.maxHistory * 2) {
            sessions[userId] = sessions[userId].slice(-config.maxHistory * 2);
        }

        res.json({ reply: reply });

    } catch (error) {
        console.error("Error:", error.message);
        res.status(502).json({ error: "সার্ভার ব্যস্ত, একটু পরে আবার চেষ্টা করুন।" });
    }
});

app.get('/history', (req, res) => {
    const userId = req.query.user_id || "default";
    res.json({ history: sessions[userId] || [] });
});

app.listen(port, () => {
    console.log(`🚀 নবিতা AI Backend চলছে — port ${port}`);
});
                                                          
