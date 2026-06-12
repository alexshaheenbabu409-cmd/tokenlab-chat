const axios = require('axios');

const config = {
    apiKey: process.env.TOKENLAB_API_KEY,
    tavilyKey: process.env.TAVILY_API_KEY,
    systemPrompt: process.env.SYSTEM_PROMPT || "",
    defaultModel: "deepseek-v4-flash",
    apiBase: "https://api.tokenlab.sh/v1",
    maxHistory: 20
};

async function searchInternet(query) {
    if (!config.tavilyKey) return "";
    try {
        const response = await axios.post('https://api.tavily.com/search', {
            api_key: config.tavilyKey,
            query: query,
            search_depth: "basic",
            max_results: 3
        });
        const results = response.data.results;
        if (!results || results.length === 0) return "";

        let context = "Here is the latest live internet data for context:\n";
        results.forEach((res, i) => {
            context += `[${i + 1}] Title: ${res.title}\nContent: ${res.content}\nURL: ${res.url}\n\n`;
        });
        return context;
    } catch (error) {
        return "";
    }
}

function shouldSearchQuery(message) {
    const msg = message.toLowerCase();
    const keywords = ["আজকের", "এখনকার", "খবর", "news", "weather", "today", "current", "live", "latest", "সাম্প্রতিক"];
    return keywords.some(keyword => msg.includes(keyword));
}

module.exports = { config, searchInternet, shouldSearchQuery };
