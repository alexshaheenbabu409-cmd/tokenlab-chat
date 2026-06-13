const axios = require('axios');

const config = {
    apiKey: process.env.TOKENLAB_API_KEY,
    tavilyKey: process.env.TAVILY_API_KEY,
    systemPrompt: process.env.SYSTEM_PROMPT || "You are a helpful AI assistant.",
    defaultModel: "deepseek-v4-flash",
    apiBase: "https://api.tokenlab.sh/v1",
    maxHistory: 10  // শেষ ১০টা message মনে রাখবে
};

async function searchInternet(query) {
    if (!config.tavilyKey) return "Search API key is missing.";
    try {
        const response = await axios.post('https://api.tavily.com/search', {
            api_key: config.tavilyKey,
            query: query,
            search_depth: "basic",  // advanced → basic করা হয়েছে, অনেক fast হবে
            max_results: 3,         // 5 → 3 করা হয়েছে, কম token খাবে
            include_answer: true
        });

        const results = response.data.results;
        const tavilyAnswer = response.data.answer;

        if (!results || results.length === 0) return "No relevant information found.";

        let context = "Latest Internet Data:\n";
        if (tavilyAnswer) {
            context += `Summary: ${tavilyAnswer}\n\n`;
        }
        results.forEach((res, i) => {
            context += `[${i + 1}] ${res.title}\n${res.content}\n\n`;
        });
        return context;

    } catch (error) {
        return `Search failed: ${error.message}`;
    }
}

module.exports = { config, searchInternet };
