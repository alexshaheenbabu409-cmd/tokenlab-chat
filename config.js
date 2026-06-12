const axios = require('axios');

const config = {
    apiKey: process.env.TOKENLAB_API_KEY,
    tavilyKey: process.env.TAVILY_API_KEY,
    systemPrompt: process.env.SYSTEM_PROMPT || "You are a helpful, smart AI assistant. Answer accurately.",
    defaultModel: "deepseek-v4-flash",
    apiBase: "https://api.tokenlab.sh/v1",
    maxHistory: 20
};

// এই ফাংশনটি এখন এআই-এর একটি "টুল" হিসেবে কাজ করবে
async function searchInternet(query) {
    if (!config.tavilyKey) return "Search API key is missing.";
    try {
        const response = await axios.post('https://api.tavily.com/search', {
            api_key: config.tavilyKey,
            query: query,
            search_depth: "advanced", // উন্নত সার্চ
            max_results: 5,           // বেশি রেজাল্ট
            include_answer: true
        });
        
        const results = response.data.results;
        const tavilyAnswer = response.data.answer;
        
        if (!results || results.length === 0) return "No relevant recent information found on the internet.";

        let context = "Latest Internet Data:\n";
        if (tavilyAnswer) {
            context += `Summary Answer: ${tavilyAnswer}\n\n`;
        }
        results.forEach((res, i) => {
            context += `[${i + 1}] Title: ${res.title}\nContent: ${res.content}\nURL: ${res.url}\n\n`;
        });
        return context;
    } catch (error) {
        return `Search failed: ${error.message}`;
    }
}

module.exports = { config, searchInternet };
          
