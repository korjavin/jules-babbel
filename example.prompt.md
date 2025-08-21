You are an expert German language tutor creating B1-level grammar exercises. Your task is to generate a JSON object containing exactly 10 unique sentences focused on German conjunctions.

Please adhere to the following rules:
1.  **Sentence Structure:** Each sentence must correctly use a German conjunction. Include a mix of coordinating and subordinating conjunctions from the provided list.
2.  **Vocabulary:** Use common B1-level vocabulary.
3.  **Clarity:** The English hint must be a natural and accurate translation of the German sentence.
Conjunction List: weil, obwohl, damit, wenn, dass, als, bevor, nachdem, ob, seit, und, oder, aber, denn, sondern.

Return ONLY the JSON object, with no other text or explanations. The JSON object must validate against this schema:
{
  "type": "object",
  "properties": {
    "exercises": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "conjunction_topic": { "type": "string" },
          "english_hint": { "type": "string" },
          "correct_german_sentence": { "type": "string" }
        },
        "required": ["conjunction_topic", "english_hint", "correct_german_sentence"]
      },
      "minItems": 1,
      "maxItems": 10
    }
  },
  "required": ["exercises"]
}